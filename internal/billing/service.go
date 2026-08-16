package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

type Service struct {
	store *db.Store
	log   *slog.Logger
}

func New(store *db.Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, log: log}
}

func (s *Service) queries() *sqlcdb.Queries {
	return s.store.Queries
}

func (s *Service) clientDeleted(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID) (bool, error) {
	if q == nil {
		q = s.queries()
	}
	cl, err := q.GetClientByID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	return cl.Status == sqlcdb.ClientStatusDeleted, nil
}

func HoldKey(messageID uuid.UUID) string   { return "hold:sms:" + messageID.String() }
func DebitKey(messageID uuid.UUID) string  { return "debit:sms:" + messageID.String() }
func ReleaseKey(messageID uuid.UUID) string { return "release:sms:" + messageID.String() }

func (s *Service) EnsureWallet(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID) (sqlcdb.Wallet, error) {
	if q == nil {
		q = s.queries()
	}
	return q.InsertWallet(ctx, sqlcdb.InsertWalletParams{ClientID: clientID, Currency: "RUB"})
}

func (s *Service) ReserveForMessage(ctx context.Context, q *sqlcdb.Queries, msg sqlcdb.SmsMessage) error {
	if q == nil {
		return errors.New("reserve requires transaction queries")
	}
	if msg.ClientID == nil {
		return wrap(ErrValidation, "validation", "message has no client")
	}
	if gone, err := s.clientDeleted(ctx, q, *msg.ClientID); err != nil {
		return err
	} else if gone {
		return nil
	}
	if msg.UnitSellPrice == nil || msg.BilledSegments == nil || msg.TariffPlanID == nil || msg.TariffPlanCode == nil || msg.Currency == nil {
		return wrap(ErrPriceSnapshotMissing, "price_snapshot_missing", "price snapshot missing")
	}
	amount := msg.UnitSellPrice.Mul(decimal.NewFromInt(int64(*msg.BilledSegments)))
	if err := mustPositive(amount, "hold_amount"); err != nil {
		return err
	}
	key := HoldKey(msg.ID)
	if existing, err := q.GetWalletTxByIdempotency(ctx, sqlcdb.GetWalletTxByIdempotencyParams{
		ClientID:       *msg.ClientID,
		IdempotencyKey: &key,
	}); err == nil && existing.ID != uuid.Nil {
		return nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	w, err := q.LockWalletByClientID(ctx, *msg.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wrap(ErrWalletNotFound, "wallet_not_found", "wallet not found")
		}
		return err
	}
	avail, held, err := applyHold(w.AvailableBalance, w.HeldBalance, amount)
	if err != nil {
		return err
	}
	if _, err := q.UpdateWalletBalances(ctx, sqlcdb.UpdateWalletBalancesParams{
		AvailableBalance: avail,
		HeldBalance:      held,
		ID:               w.ID,
		Version:          w.Version,
	}); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{
		"sms_message_id":    msg.ID.String(),
		"to_msisdn":         msg.ToMsisdn,
		"billed_segments":   *msg.BilledSegments,
		"unit_sell_price":   moneyString(*msg.UnitSellPrice),
		"tariff_plan_id":    msg.TariffPlanID.String(),
		"tariff_plan_code":  *msg.TariffPlanCode,
		"price_source":      "message_snapshot",
		"campaign_id":       uuidPtr(msg.CampaignID),
	})
	desc := "Reserve SMS"
	if _, err := q.InsertWalletTransaction(ctx, sqlcdb.InsertWalletTransactionParams{
		WalletID:              w.ID,
		ClientID:              *msg.ClientID,
		Type:                  sqlcdb.WalletTxTypeHOLD,
		Amount:                amount,
		Currency:              trimCurrency(*msg.Currency),
		BalanceAfterAvailable: &avail,
		BalanceAfterHeld:      &held,
		SmsMessageID:          &msg.ID,
		IdempotencyKey:        &key,
		Description:           &desc,
		Metadata:              meta,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) CaptureForMessage(ctx context.Context, messageID uuid.UUID) error {
	return s.settle(ctx, messageID, true)
}

func (s *Service) ReleaseForMessage(ctx context.Context, messageID uuid.UUID) error {
	return s.settle(ctx, messageID, false)
}

func (s *Service) settle(ctx context.Context, messageID uuid.UUID, capture bool) error {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	msg, err := q.GetSmsMessageByID(ctx, messageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	if msg.ClientID == nil || msg.UnitSellPrice == nil {
		return tx.Commit(ctx)
	}
	gone, err := s.clientDeleted(ctx, q, *msg.ClientID)
	if err != nil {
		return err
	}
	if gone {
		return tx.Commit(ctx)
	}

	key := DebitKey(messageID)
	if !capture {
		key = ReleaseKey(messageID)
	}
	if existing, err := q.GetWalletTxByIdempotency(ctx, sqlcdb.GetWalletTxByIdempotencyParams{
		ClientID:       *msg.ClientID,
		IdempotencyKey: &key,
	}); err == nil && existing.ID != uuid.Nil {
		action := sqlcdb.BillingActionCapture
		if !capture {
			action = sqlcdb.BillingActionRelease
		}
		_, _ = q.SetSmsMessageBillingAction(ctx, sqlcdb.SetSmsMessageBillingActionParams{
			BillingAction: sqlcdb.NullBillingAction{BillingAction: action, Valid: true},
			ID:            messageID,
		})
		return tx.Commit(ctx)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	hold, err := q.GetOpenHoldForMessage(ctx, &messageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}

	w, err := q.LockWalletByClientID(ctx, *msg.ClientID)
	if err != nil {
		return err
	}
	var avail, held decimal.Decimal
	var txType sqlcdb.WalletTxType
	desc := "Capture SMS"
	if capture {
		avail, held, err = applyDebitFromHold(w.AvailableBalance, w.HeldBalance, hold.Amount)
		txType = sqlcdb.WalletTxTypeDEBIT
	} else {
		avail, held, err = applyRelease(w.AvailableBalance, w.HeldBalance, hold.Amount)
		txType = sqlcdb.WalletTxTypeRELEASE
		desc = "Release SMS"
	}
	if err != nil {
		return err
	}
	if _, err := q.UpdateWalletBalances(ctx, sqlcdb.UpdateWalletBalancesParams{
		AvailableBalance: avail,
		HeldBalance:      held,
		ID:               w.ID,
		Version:          w.Version,
	}); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{
		"sms_message_id":  messageID.String(),
		"to_msisdn":       msg.ToMsisdn,
		"billed_segments": msg.BilledSegments,
		"unit_sell_price": moneyStringPtr(msg.UnitSellPrice),
		"price_source":    "message_snapshot",
	})
	holdID := hold.ID
	if _, err := q.InsertWalletTransaction(ctx, sqlcdb.InsertWalletTransactionParams{
		WalletID:              w.ID,
		ClientID:              *msg.ClientID,
		Type:                  txType,
		Amount:                hold.Amount,
		Currency:              hold.Currency,
		BalanceAfterAvailable: &avail,
		BalanceAfterHeld:      &held,
		RelatedHoldID:         &holdID,
		SmsMessageID:          &messageID,
		IdempotencyKey:        &key,
		Description:           &desc,
		Metadata:              meta,
	}); err != nil {
		return err
	}
	action := sqlcdb.BillingActionCapture
	if !capture {
		action = sqlcdb.BillingActionRelease
	}
	if _, err := q.SetSmsMessageBillingAction(ctx, sqlcdb.SetSmsMessageBillingActionParams{
		BillingAction: sqlcdb.NullBillingAction{BillingAction: action, Valid: true},
		ID:            messageID,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type TopUpInput struct {
	ClientID       uuid.UUID
	Amount         string
	Comment        string
	IdempotencyKey string
	CreatedBy      *uuid.UUID
}

func (s *Service) TopUp(ctx context.Context, in TopUpInput) error {
	amt, err := money(in.Amount)
	if err != nil {
		return err
	}
	if err := mustPositive(amt, "amount"); err != nil {
		return err
	}
	if in.IdempotencyKey == "" {
		return fmt.Errorf("%w: idempotency_key required", ErrValidation)
	}
	return s.credit(ctx, in.ClientID, amt, "CREDIT", "credit", in.Comment, "topup:"+in.ClientID.String()+":"+in.IdempotencyKey, in.CreatedBy, false)
}

type AdjustInput struct {
	ClientID       uuid.UUID
	Amount         string
	Direction      string
	Comment        string
	IdempotencyKey string
	AllowNegative  bool
	CreatedBy      *uuid.UUID
}

func (s *Service) Adjust(ctx context.Context, in AdjustInput) error {
	amt, err := money(in.Amount)
	if err != nil {
		return err
	}
	if err := mustPositive(amt, "amount"); err != nil {
		return err
	}
	if in.Comment == "" {
		return fmt.Errorf("%w: comment required", ErrValidation)
	}
	if in.IdempotencyKey == "" {
		return fmt.Errorf("%w: idempotency_key required", ErrValidation)
	}
	dir := in.Direction
	if dir != "debit" {
		dir = "credit"
	}
	return s.credit(ctx, in.ClientID, amt, "ADJUSTMENT", dir, in.Comment, "adjustment:"+in.ClientID.String()+":"+in.IdempotencyKey, in.CreatedBy, in.AllowNegative)
}

func (s *Service) credit(ctx context.Context, clientID uuid.UUID, amt decimal.Decimal, kind, direction, comment, key string, createdBy *uuid.UUID, allowNeg bool) error {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)
	if existing, err := q.GetWalletTxByIdempotency(ctx, sqlcdb.GetWalletTxByIdempotencyParams{
		ClientID:       clientID,
		IdempotencyKey: &key,
	}); err == nil && existing.ID != uuid.Nil {
		return tx.Commit(ctx)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	w, err := q.LockWalletByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wrap(ErrWalletNotFound, "wallet_not_found", "wallet not found")
		}
		return err
	}
	var avail decimal.Decimal
	var txType sqlcdb.WalletTxType
	if kind == "CREDIT" {
		avail = w.AvailableBalance.Add(amt)
		txType = sqlcdb.WalletTxTypeCREDIT
	} else {
		avail, err = applyAdjustment(w.AvailableBalance, amt, direction == "debit", allowNeg)
		if err != nil {
			return err
		}
		txType = sqlcdb.WalletTxTypeADJUSTMENT
	}
	held := w.HeldBalance
	if _, err := q.UpdateWalletBalances(ctx, sqlcdb.UpdateWalletBalancesParams{
		AvailableBalance: avail,
		HeldBalance:      held,
		ID:               w.ID,
		Version:          w.Version,
	}); err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]any{"direction": direction, "comment": comment})
	desc := comment
	if _, err := q.InsertWalletTransaction(ctx, sqlcdb.InsertWalletTransactionParams{
		WalletID:              w.ID,
		ClientID:              clientID,
		Type:                  txType,
		Amount:                amt,
		Currency:              trimCurrency(w.Currency),
		BalanceAfterAvailable: &avail,
		BalanceAfterHeld:      &held,
		IdempotencyKey:        &key,
		Description:           &desc,
		Metadata:              meta,
		CreatedBy:             createdBy,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReapOpenHolds(ctx context.Context, limit int32) (int, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.queries().ListOpenHoldMessages(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if row.SmsMessageID == nil {
			continue
		}
		action := settleAction(row)
		var e error
		switch action {
		case "capture":
			e = s.CaptureForMessage(ctx, *row.SmsMessageID)
		case "release":
			e = s.ReleaseForMessage(ctx, *row.SmsMessageID)
		default:
			continue
		}
		if e != nil && s.log != nil {
			s.log.Error("billing reaper", "message_id", row.SmsMessageID, "err", e)
			continue
		}
		n++
	}
	return n, nil
}

func settleAction(row sqlcdb.ListOpenHoldMessagesRow) string {
	if row.BillingAction.Valid {
		if row.BillingAction.BillingAction == sqlcdb.BillingActionCapture {
			return "capture"
		}
		if row.BillingAction.BillingAction == sqlcdb.BillingActionRelease {
			return "release"
		}
	}
	if row.AcceptedAt != nil {
		return "capture"
	}
	if row.MessageStatus == sqlcdb.SmsStatusFailed && row.AcceptedAt == nil &&
		row.JobStatus.Valid && row.JobStatus.SendJobStatus == sqlcdb.SendJobStatusDead {
		return "release"
	}
	return ""
}

func uuidPtr(id *uuid.UUID) any {
	if id == nil {
		return nil
	}
	return id.String()
}

func moneyStringPtr(d *decimal.Decimal) string {
	if d == nil {
		return ""
	}
	return moneyString(*d)
}

func CodeOf(err error) string {
	var be *Error
	if errors.As(err, &be) && be.Code != "" {
		return be.Code
	}
	switch {
	case errors.Is(err, ErrInsufficientFunds):
		return "insufficient_funds"
	case errors.Is(err, ErrTariffNotConfigured):
		return "tariff_not_configured"
	case errors.Is(err, ErrInvalidTariff):
		return "invalid_tariff"
	case errors.Is(err, ErrPriceSnapshotMissing):
		return "price_snapshot_missing"
	case errors.Is(err, ErrWalletNotFound):
		return "wallet_not_found"
	case errors.Is(err, ErrNegativeBalance):
		return "negative_balance_forbidden"
	case errors.Is(err, ErrValidation), errors.Is(err, ErrInvalidAmount):
		return "validation"
	default:
		return "internal"
	}
}

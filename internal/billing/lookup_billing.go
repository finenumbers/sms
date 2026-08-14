package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const walletVersionRetries = 5

func (s *Service) EstimateLookup(ctx context.Context, clientID uuid.UUID, checkType sqlcdb.LookupCheckType, unitCount int64) (Estimate, error) {
	if unitCount < 1 {
		return Estimate{}, wrap(ErrValidation, "validation", "unit count must be >= 1")
	}
	product, err := ProductForCheckType(checkType)
	if err != nil {
		return Estimate{}, err
	}
	tariff, err := s.ResolveTariff(ctx, nil, clientID, product)
	if err != nil {
		return Estimate{}, err
	}
	total := tariff.SellPrice.Mul(decimal.NewFromInt(unitCount))
	return Estimate{
		Product:        product,
		Segments:       int(unitCount),
		UnitSellPrice:  tariff.SellPrice,
		Total:          total,
		Currency:       tariff.Currency,
		TariffPlanID:   tariff.PlanID,
		TariffPlanCode: tariff.PlanCode,
	}, nil
}

func (s *Service) AssertLookupAssignment(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID, checkType sqlcdb.LookupCheckType) error {
	product, err := ProductForCheckType(checkType)
	if err != nil {
		return err
	}
	_, err = s.ResolveTariff(ctx, q, clientID, product)
	return err
}

// ReserveForLookupJob HOLDs unit_sell_price × item_count. Caller must pass transactional q
// after inserting job+items. SMS settle() must not be used for this hold.
func (s *Service) ReserveForLookupJob(ctx context.Context, q *sqlcdb.Queries, job sqlcdb.LookupJob) error {
	if q == nil {
		return errors.New("reserve lookup job requires transaction queries")
	}
	if job.ItemCount < 1 {
		return wrap(ErrValidation, "validation", "lookup job has no items")
	}
	if job.UnitSellPrice == nil || job.TariffPlanID == nil || job.TariffPlanCode == nil {
		return wrap(ErrPriceSnapshotMissing, "price_snapshot_missing", "price snapshot missing")
	}
	amount := job.UnitSellPrice.Mul(decimal.NewFromInt(int64(job.ItemCount)))
	if err := mustPositive(amount, "hold_amount"); err != nil {
		return err
	}
	key := LookupHoldKey(job.ID)
	if existing, err := q.GetWalletTxByIdempotency(ctx, sqlcdb.GetWalletTxByIdempotencyParams{
		ClientID:       job.ClientID,
		IdempotencyKey: &key,
	}); err == nil && existing.ID != uuid.Nil {
		return nil
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	w, err := q.LockWalletByClientID(ctx, job.ClientID)
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
		"lookup_job_id":    job.ID.String(),
		"check_type":       string(job.CheckType),
		"item_count":       job.ItemCount,
		"unit_sell_price":  moneyString(*job.UnitSellPrice),
		"tariff_plan_id":   job.TariffPlanID.String(),
		"tariff_plan_code": *job.TariffPlanCode,
		"price_source":     "job_snapshot",
	})
	desc := "Reserve lookup job"
	jobID := job.ID
	if _, err := q.InsertWalletTransaction(ctx, sqlcdb.InsertWalletTransactionParams{
		WalletID:              w.ID,
		ClientID:              job.ClientID,
		Type:                  sqlcdb.WalletTxTypeHOLD,
		Amount:                amount,
		Currency:              trimCurrency(job.Currency),
		BalanceAfterAvailable: &avail,
		BalanceAfterHeld:      &held,
		LookupJobID:           &jobID,
		IdempotencyKey:        &key,
		Description:           &desc,
		Metadata:              meta,
	}); err != nil {
		return err
	}
	return nil
}

func (s *Service) CaptureForLookupItem(ctx context.Context, itemID uuid.UUID) error {
	return s.settleLookupItem(ctx, itemID, true)
}

func (s *Service) ReleaseForLookupItem(ctx context.Context, itemID uuid.UUID) error {
	return s.settleLookupItem(ctx, itemID, false)
}

func (s *Service) settleLookupItem(ctx context.Context, itemID uuid.UUID, capture bool) error {
	var last error
	for attempt := 0; attempt < walletVersionRetries; attempt++ {
		last = s.settleLookupItemOnce(ctx, itemID, capture)
		if last == nil || !errors.Is(last, pgx.ErrNoRows) {
			return last
		}
	}
	if last == nil {
		return wrap(ErrInvalidAmount, "invalid_amount", "wallet version conflict")
	}
	return last
}

func (s *Service) settleLookupItemOnce(ctx context.Context, itemID uuid.UUID, capture bool) error {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	item, err := q.GetLookupItem(ctx, itemID)
	if err != nil {
		return err
	}
	if item.BillingAction.Valid {
		want := sqlcdb.BillingActionCapture
		if !capture {
			want = sqlcdb.BillingActionRelease
		}
		if item.BillingAction.BillingAction == want {
			return tx.Commit(ctx)
		}
		return wrap(ErrInvalidAmount, "invalid_amount", "lookup item already settled with a different action")
	}
	if item.UnitSellPrice == nil {
		return wrap(ErrPriceSnapshotMissing, "price_snapshot_missing", "item price snapshot missing")
	}
	share := *item.UnitSellPrice
	if err := mustPositive(share, "item_amount"); err != nil {
		return err
	}

	key := LookupDebitKey(itemID)
	if !capture {
		key = LookupReleaseKey(itemID)
	}
	if existing, err := q.GetWalletTxByIdempotency(ctx, sqlcdb.GetWalletTxByIdempotencyParams{
		ClientID:       item.ClientID,
		IdempotencyKey: &key,
	}); err == nil && existing.ID != uuid.Nil {
		if err := markLookupItemPosted(q, ctx, item.ID, capture, share); err != nil {
			return err
		}
		return tx.Commit(ctx)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	jobID := item.JobID
	hold, err := q.GetLookupJobHold(ctx, &jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return wrap(ErrHoldNotFound, "hold_not_found", "lookup job hold not found")
		}
		return err
	}
	settled, err := q.SumSettledAgainstHold(ctx, &hold.ID)
	if err != nil {
		return err
	}
	remaining, err := RemainingOnHold(hold.Amount, settled)
	if err != nil {
		return err
	}
	if remaining.LessThan(share) {
		return wrap(ErrInvalidAmount, "invalid_amount",
			fmt.Sprintf("hold remaining %s less than item share %s", moneyString(remaining), moneyString(share)))
	}

	w, err := q.LockWalletByClientID(ctx, item.ClientID)
	if err != nil {
		return err
	}
	var avail, held decimal.Decimal
	var txType sqlcdb.WalletTxType
	desc := "Capture lookup"
	if capture {
		avail, held, err = applyDebitFromHold(w.AvailableBalance, w.HeldBalance, share)
		txType = sqlcdb.WalletTxTypeDEBIT
	} else {
		avail, held, err = applyRelease(w.AvailableBalance, w.HeldBalance, share)
		txType = sqlcdb.WalletTxTypeRELEASE
		desc = "Release lookup"
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
		"lookup_job_id":   item.JobID.String(),
		"lookup_item_id":  item.ID.String(),
		"phone_e164":      item.PhoneE164,
		"unit_sell_price": moneyString(share),
		"price_source":    "item_snapshot",
	})
	holdID := hold.ID
	itemIDCopy := item.ID
	if _, err := q.InsertWalletTransaction(ctx, sqlcdb.InsertWalletTransactionParams{
		WalletID:              w.ID,
		ClientID:              item.ClientID,
		Type:                  txType,
		Amount:                share,
		Currency:              hold.Currency,
		BalanceAfterAvailable: &avail,
		BalanceAfterHeld:      &held,
		RelatedHoldID:         &holdID,
		LookupJobID:           &jobID,
		LookupItemID:          &itemIDCopy,
		IdempotencyKey:        &key,
		Description:           &desc,
		Metadata:              meta,
	}); err != nil {
		return err
	}

	if err := markLookupItemPosted(q, ctx, item.ID, capture, share); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func markLookupItemPosted(q *sqlcdb.Queries, ctx context.Context, itemID uuid.UUID, capture bool, share decimal.Decimal) error {
	action := sqlcdb.BillingActionCapture
	var actual *decimal.Decimal
	if capture {
		actual = &share
	} else {
		action = sqlcdb.BillingActionRelease
	}
	return q.SetLookupItemBillingAction(ctx, sqlcdb.SetLookupItemBillingActionParams{
		BillingAction: sqlcdb.NullBillingAction{BillingAction: action, Valid: true},
		ActualCost:    actual,
		ID:            itemID,
	})
}

func (s *Service) ReleaseLookupJobRemainder(ctx context.Context, jobID uuid.UUID) error {
	var last error
	for attempt := 0; attempt < walletVersionRetries; attempt++ {
		last = s.releaseLookupJobRemainderOnce(ctx, jobID)
		if last == nil || !errors.Is(last, pgx.ErrNoRows) {
			return last
		}
	}
	return last
}

func (s *Service) releaseLookupJobRemainderOnce(ctx context.Context, jobID uuid.UUID) error {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	blocking, err := q.CountLookupItemsBlockingRemainder(ctx, jobID)
	if err != nil {
		return err
	}
	if LookupRemainderBlocked(blocking) {
		return tx.Commit(ctx)
	}

	hold, err := q.GetLookupJobHold(ctx, &jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return tx.Commit(ctx)
		}
		return err
	}
	settled, err := q.SumSettledAgainstHold(ctx, &hold.ID)
	if err != nil {
		return err
	}
	remaining, err := RemainingOnHold(hold.Amount, settled)
	if err != nil {
		return err
	}
	if !HoldIsOpen(remaining) {
		return tx.Commit(ctx)
	}

	key := LookupReleaseRemainderKey(jobID)
	if existing, err := q.GetWalletTxByIdempotency(ctx, sqlcdb.GetWalletTxByIdempotencyParams{
		ClientID:       hold.ClientID,
		IdempotencyKey: &key,
	}); err == nil && existing.ID != uuid.Nil {
		return tx.Commit(ctx)
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	w, err := q.LockWalletByClientID(ctx, hold.ClientID)
	if err != nil {
		return err
	}
	avail, held, err := applyRelease(w.AvailableBalance, w.HeldBalance, remaining)
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
		"lookup_job_id": jobID.String(),
		"price_source":  "hold_remainder",
	})
	desc := "Release lookup remainder"
	holdID := hold.ID
	if _, err := q.InsertWalletTransaction(ctx, sqlcdb.InsertWalletTransactionParams{
		WalletID:              w.ID,
		ClientID:              hold.ClientID,
		Type:                  sqlcdb.WalletTxTypeRELEASE,
		Amount:                remaining,
		Currency:              hold.Currency,
		BalanceAfterAvailable: &avail,
		BalanceAfterHeld:      &held,
		RelatedHoldID:         &holdID,
		LookupJobID:           &jobID,
		IdempotencyKey:        &key,
		Description:           &desc,
		Metadata:              meta,
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReapOpenLookupHolds(ctx context.Context, limit int32) (int, error) {
	if limit < 1 {
		limit = 50
	}
	rows, err := s.queries().ListOpenLookupJobHolds(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		if row.LookupJobID == nil {
			continue
		}
		items, err := s.queries().ListUnsettledTerminalLookupItems(ctx, *row.LookupJobID)
		if err != nil {
			if s.log != nil {
				s.log.Error("lookup reaper list items", "job_id", row.LookupJobID, "err", err)
			}
			continue
		}
		for _, item := range items {
			switch LookupItemSettleAction(item) {
			case "capture":
				if e := s.CaptureForLookupItem(ctx, item.ID); e != nil && s.log != nil {
					s.log.Error("lookup reaper capture", "item_id", item.ID, "err", e)
				} else if e == nil {
					n++
				}
			case "release":
				if e := s.ReleaseForLookupItem(ctx, item.ID); e != nil && s.log != nil {
					s.log.Error("lookup reaper release", "item_id", item.ID, "err", e)
				} else if e == nil {
					n++
				}
			}
		}
		switch row.JobStatus {
		case sqlcdb.LookupJobStatusCompleted, sqlcdb.LookupJobStatusCompletedWithErrors, sqlcdb.LookupJobStatusFailed:
			if e := s.ReleaseLookupJobRemainder(ctx, *row.LookupJobID); e != nil && s.log != nil {
				s.log.Error("lookup reaper remainder", "job_id", row.LookupJobID, "err", e)
			}
		}
	}
	return n, nil
}

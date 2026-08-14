package campaigns

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/settings"
)

const (
	maxTextRunes   = 1000
	maxRecipients  = 100000
	recipientChunk = 2000
)

var (
	ErrValidation  = errors.New("validation")
	ErrNotFound    = errors.New("not found")
	ErrNotAssigned = errors.New("from number is not assigned to this client")
	ErrFrozen      = errors.New("campaign from and text are immutable after start")
	ErrNotDraft    = errors.New("campaign is not a draft")
	ErrEmpty       = errors.New("campaign has no recipients")
	ErrConflict    = errors.New("campaign cannot be changed in this status")
	ErrTooMany     = errors.New("too many recipients")
)

type DirectionPolicy interface {
	Get(ctx context.Context) (settings.Public, error)
}

type Service struct {
	store   *db.Store
	dirs    DirectionPolicy
	billing *billing.Service
}

func New(store *db.Store, dirs DirectionPolicy, bill *billing.Service) *Service {
	return &Service{store: store, dirs: dirs, billing: bill}
}

type CreateInput struct {
	ClientID  uuid.UUID
	CreatedBy *uuid.UUID
	From      string
	Text      string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (sqlcdb.SmsCampaign, error) {
	from, text, err := s.validateFromText(ctx, in.ClientID, in.From, in.Text)
	if err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	return s.store.Queries.InsertCampaign(ctx, sqlcdb.InsertCampaignParams{
		ClientID:   in.ClientID,
		FromMsisdn: from,
		Text:       text,
		CreatedBy:  in.CreatedBy,
	})
}

type PatchInput struct {
	From *string
	Text *string
}

func (s *Service) Patch(ctx context.Context, clientID, id uuid.UUID, in PatchInput) (sqlcdb.SmsCampaign, error) {
	cur, err := s.get(ctx, clientID, id)
	if err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	if cur.Status != sqlcdb.CampaignStatusDraft {
		return sqlcdb.SmsCampaign{}, ErrFrozen
	}
	from, text := cur.FromMsisdn, cur.Text
	if in.From != nil {
		from = *in.From
	}
	if in.Text != nil {
		text = *in.Text
	}
	from, text, err = s.validateFromText(ctx, clientID, from, text)
	if err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	out, err := s.store.Queries.UpdateCampaignDraft(ctx, sqlcdb.UpdateCampaignDraftParams{
		FromMsisdn: &from,
		Text:       &text,
		ID:         id,
		ClientID:   clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrFrozen
		}
		return sqlcdb.SmsCampaign{}, err
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.SmsCampaign, sqlcdb.CampaignRecipientStatsRow, error) {
	c, err := s.get(ctx, clientID, id)
	if err != nil {
		return sqlcdb.SmsCampaign{}, sqlcdb.CampaignRecipientStatsRow{}, err
	}
	st, err := s.store.Queries.CampaignRecipientStats(ctx, id)
	if err != nil {
		return sqlcdb.SmsCampaign{}, sqlcdb.CampaignRecipientStatsRow{}, err
	}
	return c, st, nil
}

type ListFilter struct {
	Status *sqlcdb.CampaignStatus
	Limit  int32
	Offset int32
}

func (s *Service) List(ctx context.Context, clientID uuid.UUID, f ListFilter) ([]sqlcdb.SmsCampaign, error) {
	limit, offset := f.Limit, f.Offset
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	arg := sqlcdb.ListCampaignsForClientParams{
		ClientID:   clientID,
		PageLimit:  limit,
		PageOffset: offset,
	}
	if f.Status != nil {
		arg.Status = sqlcdb.NullCampaignStatus{CampaignStatus: *f.Status, Valid: true}
	}
	return s.store.Queries.ListCampaignsForClient(ctx, arg)
}

func (s *Service) DeleteDraft(ctx context.Context, clientID, id uuid.UUID) error {
	n, err := s.store.Queries.DeleteCampaignDraft(ctx, sqlcdb.DeleteCampaignDraftParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		_, err := s.get(ctx, clientID, id)
		if err != nil {
			return err
		}
		return ErrNotDraft
	}
	return nil
}

type AddResult struct {
	Added      int64        `json:"added"`
	Duplicates int          `json:"duplicates"`
	Invalid    []InvalidRow `json:"invalid,omitempty"`
	Total      int32        `json:"total"`
	Encoding   string       `json:"encoding,omitempty"`
}

func (s *Service) AddRecipients(ctx context.Context, clientID, id uuid.UUID, msisdns []string, invalid []InvalidRow) (AddResult, error) {
	out := AddResult{Invalid: invalid}
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return AddResult{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	c, err := q.GetCampaignForClientForUpdate(ctx, sqlcdb.GetCampaignForClientForUpdateParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AddResult{}, ErrNotFound
		}
		return AddResult{}, err
	}
	if c.Status != sqlcdb.CampaignStatusDraft {
		return AddResult{}, ErrFrozen
	}

	existing, err := q.ListCampaignRecipientMSISDNs(ctx, id)
	if err != nil {
		return AddResult{}, err
	}
	have := make(map[string]struct{}, len(existing))
	for _, n := range existing {
		have[n] = struct{}{}
	}

	intOut := s.intOut(ctx)
	fresh := make([]string, 0, len(msisdns))
	dup := 0
	for _, n := range msisdns {
		if _, ok := have[n]; ok {
			dup++
			continue
		}
		dest, err := msisdn.NormalizeDest(n)
		if err != nil {
			out.Invalid = append(out.Invalid, InvalidRow{Value: n, Error: err.Error()})
			continue
		}
		if dest.International && !intOut {
			out.Invalid = append(out.Invalid, InvalidRow{Value: n, Error: "international SMS is disabled"})
			continue
		}
		have[n] = struct{}{}
		fresh = append(fresh, n)
	}
	if int32(len(existing))+int32(len(fresh)) > maxRecipients {
		return AddResult{}, fmt.Errorf("%w: max %d", ErrTooMany, maxRecipients)
	}

	var added int64
	for i := 0; i < len(fresh); i += recipientChunk {
		end := i + recipientChunk
		if end > len(fresh) {
			end = len(fresh)
		}
		n, err := q.InsertCampaignRecipients(ctx, sqlcdb.InsertCampaignRecipientsParams{
			CampaignID: id,
			Msisdns:    fresh[i:end],
		})
		if err != nil {
			return AddResult{}, err
		}
		added += n
	}
	total, err := q.CountCampaignRecipients(ctx, id)
	if err != nil {
		return AddResult{}, err
	}
	if err := q.SetCampaignTotal(ctx, sqlcdb.SetCampaignTotalParams{
		TotalCount: total,
		ID:         id,
	}); err != nil {
		return AddResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return AddResult{}, err
	}
	out.Added = added
	out.Duplicates = dup
	out.Total = total
	return out, nil
}

func (s *Service) ListRecipients(ctx context.Context, clientID, id uuid.UUID, limit, offset int32) ([]sqlcdb.ListCampaignRecipientRowsRow, error) {
	if _, err := s.get(ctx, clientID, id); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.store.Queries.ListCampaignRecipientRows(ctx, sqlcdb.ListCampaignRecipientRowsParams{
		CampaignID: id,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

func (s *Service) Start(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.SmsCampaign, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	c, err := q.GetCampaignForClientForUpdate(ctx, sqlcdb.GetCampaignForClientForUpdateParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrNotFound
		}
		return sqlcdb.SmsCampaign{}, err
	}
	switch c.Status {
	case sqlcdb.CampaignStatusQueued, sqlcdb.CampaignStatusRunning:
		return c, nil
	case sqlcdb.CampaignStatusDraft:
	default:
		return sqlcdb.SmsCampaign{}, ErrConflict
	}
	if c.TotalCount <= 0 {
		return sqlcdb.SmsCampaign{}, ErrEmpty
	}
	if _, err := q.GetAssignedNumberForClient(ctx, sqlcdb.GetAssignedNumberForClientParams{
		Msisdn:   c.FromMsisdn,
		ClientID: clientID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrNotAssigned
		}
		return sqlcdb.SmsCampaign{}, err
	}
	if err := s.assertCampaignAfford(ctx, clientID, c); err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	out, err := q.QueueCampaign(ctx, sqlcdb.QueueCampaignParams{ID: id, ClientID: clientID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrConflict
		}
		return sqlcdb.SmsCampaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	return out, nil
}

func (s *Service) Cancel(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.SmsCampaign, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	c, err := q.GetCampaignForClientForUpdate(ctx, sqlcdb.GetCampaignForClientForUpdateParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrNotFound
		}
		return sqlcdb.SmsCampaign{}, err
	}
	if c.Status == sqlcdb.CampaignStatusCancelled {
		if err := tx.Commit(ctx); err != nil {
			return sqlcdb.SmsCampaign{}, err
		}
		_, _ = s.store.Queries.SkipPendingForCampaign(ctx, id)
		return c, nil
	}
	out, err := q.CancelCampaign(ctx, sqlcdb.CancelCampaignParams{ID: id, ClientID: clientID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrConflict
		}
		return sqlcdb.SmsCampaign{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.SmsCampaign{}, err
	}
	_, _ = s.store.Queries.SkipPendingForCampaign(ctx, id)
	return out, nil
}

func (s *Service) get(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.SmsCampaign, error) {
	c, err := s.store.Queries.GetCampaignForClient(ctx, sqlcdb.GetCampaignForClientParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsCampaign{}, ErrNotFound
		}
		return sqlcdb.SmsCampaign{}, err
	}
	return c, nil
}

func (s *Service) validateFromText(ctx context.Context, clientID uuid.UUID, fromRaw, textRaw string) (string, string, error) {
	from, err := msisdn.NormalizeSender(fromRaw)
	if err != nil {
		return "", "", fmt.Errorf("%w: from: %s", ErrValidation, err.Error())
	}
	text := strings.TrimSpace(textRaw)
	if text == "" {
		return "", "", fmt.Errorf("%w: text required", ErrValidation)
	}
	if utf8.RuneCountInString(text) > maxTextRunes {
		return "", "", fmt.Errorf("%w: text too long", ErrValidation)
	}
	if _, err := s.store.Queries.GetAssignedNumberForClient(ctx, sqlcdb.GetAssignedNumberForClientParams{
		Msisdn:   from,
		ClientID: clientID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrNotAssigned
		}
		return "", "", err
	}
	return from, text, nil
}

func (s *Service) intOut(ctx context.Context) bool {
	if s.dirs == nil {
		return false
	}
	v, err := s.dirs.Get(ctx)
	if err != nil {
		return false
	}
	return v.SMSDirections.IntOut
}

func (s *Service) CampaignCost(ctx context.Context, clientID uuid.UUID, c sqlcdb.SmsCampaign) (CampaignCost, error) {
	out := CampaignCost{Segments: billing.SegmentCount(c.Text), Currency: "RUB"}
	if s.billing == nil {
		return CampaignCost{}, fmt.Errorf("%w: billing unavailable", ErrValidation)
	}
	var offset int32
	for {
		rows, err := s.store.Queries.ListCampaignRecipients(ctx, sqlcdb.ListCampaignRecipientsParams{
			CampaignID: c.ID,
			PageLimit:  1000,
			PageOffset: offset,
		})
		if err != nil {
			return CampaignCost{}, err
		}
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			dest, err := msisdn.NormalizeDest(r.ToMsisdn)
			if err != nil {
				continue
			}
			if dest.International {
				out.International++
			} else {
				out.Domestic++
			}
		}
		offset += int32(len(rows))
		if len(rows) < 1000 {
			break
		}
	}
	dom, err := s.bucketCost(ctx, clientID, sqlcdb.BillingProductSmsDomestic, out.Domestic, out.Segments)
	if err != nil {
		return CampaignCost{}, err
	}
	intl, err := s.bucketCost(ctx, clientID, sqlcdb.BillingProductSmsInternational, out.International, out.Segments)
	if err != nil {
		return CampaignCost{}, err
	}
	out.Total = dom.Add(intl)
	out.Billed = true
	return out, nil
}

func (s *Service) assertCampaignAfford(ctx context.Context, clientID uuid.UUID, c sqlcdb.SmsCampaign) error {
	cost, err := s.CampaignCost(ctx, clientID, c)
	if err != nil {
		return err
	}
	return s.billing.AssertCanAfford(ctx, clientID, cost.Total)
}

type CampaignCost struct {
	Domestic      int
	International int
	Segments      int
	Total         decimal.Decimal
	Currency      string
	Billed        bool
}

func (s *Service) bucketCost(ctx context.Context, clientID uuid.UUID, product sqlcdb.BillingProduct, n, seg int) (decimal.Decimal, error) {
	if n == 0 {
		return decimal.Zero, nil
	}
	t, err := s.billing.ResolveTariff(ctx, nil, clientID, product)
	if err != nil {
		return decimal.Zero, err
	}
	return t.SellPrice.Mul(decimal.NewFromInt(int64(seg))).Mul(decimal.NewFromInt(int64(n))), nil
}

func Frozen(st sqlcdb.CampaignStatus) bool {
	return st != sqlcdb.CampaignStatusDraft
}

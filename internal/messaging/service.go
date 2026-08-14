package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/settings"
)

const maxTextRunes = 1000

var (
	ErrValidation       = errors.New("validation")
	ErrNotFound         = errors.New("not found")
	ErrNotAssigned      = errors.New("from number is not assigned to this client")
	ErrInternationalOff = errors.New("international SMS is disabled")
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

type EnqueueInput struct {
	ClientID       uuid.UUID
	From           string
	To             string
	Text           string
	IdempotencyKey *string
	CampaignID     *uuid.UUID
}

func (s *Service) Enqueue(ctx context.Context, in EnqueueInput) (sqlcdb.SmsMessage, error) {
	return s.enqueue(ctx, nil, in)
}

func (s *Service) EnqueueWith(ctx context.Context, q *sqlcdb.Queries, in EnqueueInput) (sqlcdb.SmsMessage, error) {
	return s.enqueue(ctx, q, in)
}

func (s *Service) enqueue(ctx context.Context, q *sqlcdb.Queries, in EnqueueInput) (sqlcdb.SmsMessage, error) {
	from, err := msisdn.NormalizeSender(in.From)
	if err != nil {
		return sqlcdb.SmsMessage{}, fmt.Errorf("%w: from: %s", ErrValidation, err.Error())
	}
	dest, err := msisdn.NormalizeDest(in.To)
	if err != nil {
		return sqlcdb.SmsMessage{}, fmt.Errorf("%w: to: %s", ErrValidation, err.Error())
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return sqlcdb.SmsMessage{}, fmt.Errorf("%w: text required", ErrValidation)
	}
	if utf8.RuneCountInString(text) > maxTextRunes {
		return sqlcdb.SmsMessage{}, fmt.Errorf("%w: text too long", ErrValidation)
	}
	if dest.International {
		intOut := false
		if s.dirs != nil {
			if v, err := s.dirs.Get(ctx); err == nil {
				intOut = v.SMSDirections.IntOut
			}
		}
		if !intOut {
			return sqlcdb.SmsMessage{}, ErrInternationalOff
		}
	}

	lookup := s.store.Queries
	if q != nil {
		lookup = q
	}
	if _, err := lookup.GetAssignedNumberForClient(ctx, sqlcdb.GetAssignedNumberForClientParams{
		Msisdn:   from,
		ClientID: in.ClientID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsMessage{}, ErrNotAssigned
		}
		return sqlcdb.SmsMessage{}, err
	}

	var tx pgx.Tx
	if q == nil {
		var err error
		tx, err = s.store.Pool.Begin(ctx)
		if err != nil {
			return sqlcdb.SmsMessage{}, err
		}
		defer tx.Rollback(ctx)
		q = s.store.Queries.WithTx(tx)
	}

	params := sqlcdb.InsertSmsMessageParams{
		ClientID:       &in.ClientID,
		Direction:      sqlcdb.SmsDirectionOutbound,
		FromMsisdn:     from,
		ToMsisdn:       dest.MSISDN,
		Text:           text,
		CampaignID:     in.CampaignID,
		IdempotencyKey: in.IdempotencyKey,
	}

	if s.billing == nil {
		return sqlcdb.SmsMessage{}, fmt.Errorf("%w: billing unavailable", ErrValidation)
	}
	est, err := s.billing.Estimate(ctx, in.ClientID, dest, text)
	if err != nil {
		return sqlcdb.SmsMessage{}, err
	}
	params.UnitSellPrice = &est.UnitSellPrice
	seg := int32(est.Segments)
	params.BilledSegments = &seg
	params.TariffPlanID = &est.TariffPlanID
	params.TariffPlanCode = &est.TariffPlanCode
	cur := est.Currency
	params.Currency = &cur

	msg, err := q.InsertSmsMessage(ctx, params)
	if err != nil {
		if in.IdempotencyKey != nil && isUniqueViolation(err) {
			existing, getErr := q.GetSmsMessageByIdempotency(ctx, sqlcdb.GetSmsMessageByIdempotencyParams{
				ClientID:       in.ClientID,
				IdempotencyKey: in.IdempotencyKey,
			})
			if getErr == nil {
				if existing.UnitSellPrice == nil {
					return sqlcdb.SmsMessage{}, billing.ErrPriceSnapshotMissing
				}
				if err := s.billing.ReserveForMessage(ctx, q, existing); err != nil {
					return sqlcdb.SmsMessage{}, err
				}
				if tx != nil {
					if err := tx.Commit(ctx); err != nil {
						return sqlcdb.SmsMessage{}, err
					}
				}
				return existing, nil
			}
		}
		return sqlcdb.SmsMessage{}, err
	}
	if _, err := q.InsertSendJob(ctx, sqlcdb.InsertSendJobParams{
		SmsMessageID: msg.ID,
		ClientID:     &in.ClientID,
	}); err != nil {
		return sqlcdb.SmsMessage{}, err
	}
	if err := s.billing.ReserveForMessage(ctx, q, msg); err != nil {
		return sqlcdb.SmsMessage{}, err
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return sqlcdb.SmsMessage{}, err
		}
	}
	return msg, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

func (s *Service) GetForClient(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.SmsMessage, error) {
	msg, err := s.store.Queries.GetSmsMessageForClient(ctx, sqlcdb.GetSmsMessageForClientParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.SmsMessage{}, ErrNotFound
		}
		return sqlcdb.SmsMessage{}, err
	}
	return msg, nil
}

type ListFilter struct {
	Direction *sqlcdb.SmsDirection
	Limit     int32
	Offset    int32
}

func (s *Service) ListForClient(ctx context.Context, clientID uuid.UUID, f ListFilter) ([]sqlcdb.SmsMessage, error) {
	limit, offset := f.Limit, f.Offset
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	arg := sqlcdb.ListSmsMessagesForClientParams{
		ClientID:   clientID,
		PageLimit:  limit,
		PageOffset: offset,
	}
	if f.Direction != nil {
		arg.Direction = sqlcdb.NullSmsDirection{SmsDirection: *f.Direction, Valid: true}
	}
	return s.store.Queries.ListSmsMessagesForClient(ctx, arg)
}

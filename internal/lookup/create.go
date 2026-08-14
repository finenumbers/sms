package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/webhooks"
)

const createStatementTimeout = 120 * time.Second

// SetCreateStatementTimeout caps a write TX that inserts lookup items / HOLD.
// Public CSV materialize and cabinet Create must use the same budget: the
// consuming-heal TTL is derived from this value.
func SetCreateStatementTimeout(ctx context.Context, tx pgx.Tx) error {
	if tx == nil {
		return nil
	}
	_, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL statement_timeout = '%s'", createStatementTimeout))
	return err
}

type SettingsView interface {
	Get(ctx context.Context) (settings.Public, error)
}

type Service struct {
	store    *db.Store
	billing  *billing.Service
	settings SettingsView
	hooks    *webhooks.Service
	log      *slog.Logger
}

func NewService(store *db.Store, bill *billing.Service, settings SettingsView, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, billing: bill, settings: settings, log: log}
}

func (s *Service) SetWebhooks(h *webhooks.Service) {
	if s != nil {
		s.hooks = h
	}
}

type CreateInput struct {
	ClientID         uuid.UUID
	CheckType        sqlcdb.LookupCheckType
	Source           sqlcdb.LookupJobSource
	Phones           []string
	IdempotencyKey   string
	CreatedBy        *uuid.UUID
	APICredentialID  *uuid.UUID
	OriginalFilename *string
	Metadata         map[string]any
	// MaxPhones overrides Settings max_batch_phones. CSV preview submit uses max_csv_rows.
	MaxPhones    int
	PhoneCapName string
}

type CreateResult struct {
	Job                    sqlcdb.LookupJob
	Deduplicated           bool
	DeduplicatedPhoneCount int
	WorkUnits              int
}

func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	return s.create(ctx, nil, in)
}

func (s *Service) CreateWith(ctx context.Context, q *sqlcdb.Queries, in CreateInput) (CreateResult, error) {
	if q == nil {
		return CreateResult{}, errors.New("lookup create requires queries")
	}
	return s.create(ctx, q, in)
}

func (s *Service) create(ctx context.Context, qin *sqlcdb.Queries, in CreateInput) (CreateResult, error) {
	if s == nil || s.store == nil || s.billing == nil {
		return CreateResult{}, errors.New("lookup service not configured")
	}
	switch in.CheckType {
	case sqlcdb.LookupCheckTypeHlr, sqlcdb.LookupCheckTypePing:
	default:
		return CreateResult{}, wrap(ErrValidation, "validation", "check_type must be hlr or ping")
	}
	switch in.Source {
	case sqlcdb.LookupJobSourceSingle, sqlcdb.LookupJobSourceBulk, sqlcdb.LookupJobSourceApi:
	default:
		return CreateResult{}, wrap(ErrValidation, "validation", "source must be single, bulk, or api")
	}
	if in.ClientID == uuid.Nil {
		return CreateResult{}, wrap(ErrValidation, "validation", "client_id is required")
	}

	view, err := s.settings.Get(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	if !view.LookupEnabled {
		return CreateResult{}, wrap(ErrLookupDisabled, "lookup_disabled", "lookup is disabled")
	}

	client, err := s.store.Queries.GetClientByID(ctx, in.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CreateResult{}, wrap(ErrNotFound, "not_found", "client not found")
		}
		return CreateResult{}, err
	}
	if client.Status == sqlcdb.ClientStatusSuspended {
		return CreateResult{}, wrap(ErrClientSuspended, "client_suspended", "client is suspended")
	}
	if client.Status != sqlcdb.ClientStatusActive {
		return CreateResult{}, wrap(ErrNotFound, "not_found", "client not found")
	}

	if in.IdempotencyKey != "" {
		existing, err := s.store.Queries.GetLookupJobByIdempotency(ctx, sqlcdb.GetLookupJobByIdempotencyParams{
			ClientID:       in.ClientID,
			IdempotencyKey: &in.IdempotencyKey,
		})
		if err == nil {
			return CreateResult{Job: existing, Deduplicated: true, WorkUnits: int(existing.ItemCount)}, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return CreateResult{}, err
		}
	}

	maxPhones := in.MaxPhones
	if maxPhones <= 0 {
		maxPhones = int(view.LookupMaxBatchPhones)
	}
	phones, deduped, err := PreparePhones(in.Phones, string(in.Source), maxPhones, in.PhoneCapName)
	if err != nil {
		return CreateResult{}, err
	}

	est, err := s.billing.EstimateLookup(ctx, in.ClientID, in.CheckType, int64(len(phones)))
	if err != nil {
		return CreateResult{}, mapBillingErr(err)
	}

	meta := in.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	metaBytes, _ := json.Marshal(meta)
	if len(metaBytes) == 0 {
		metaBytes = []byte("{}")
	}

	var idem *string
	if in.IdempotencyKey != "" {
		idem = &in.IdempotencyKey
	}
	price := est.UnitSellPrice
	planID := est.TariffPlanID
	planCode := est.TariffPlanCode
	total := est.Total
	currency := est.Currency
	if currency == "" {
		currency = "RUB"
	}

	var (
		q  *sqlcdb.Queries
		tx pgx.Tx
	)
	if qin != nil {
		q = qin
	} else {
		begun, err := s.store.Pool.Begin(ctx)
		if err != nil {
			return CreateResult{}, err
		}
		defer begun.Rollback(ctx)
		if err := SetCreateStatementTimeout(ctx, begun); err != nil {
			return CreateResult{}, err
		}
		q = s.store.Queries.WithTx(begun)
		tx = begun
	}

	job, err := q.InsertLookupJob(ctx, sqlcdb.InsertLookupJobParams{
		ClientID:         in.ClientID,
		CheckType:        in.CheckType,
		Source:           in.Source,
		Status:           sqlcdb.LookupJobStatusQueued,
		ItemCount:        int32(len(phones)),
		UnitSellPrice:    &price,
		TariffPlanID:     &planID,
		TariffPlanCode:   &planCode,
		Currency:         currency,
		EstimatedCost:    &total,
		OriginalFilename: in.OriginalFilename,
		IdempotencyKey:   idem,
		CreatedBy:        in.CreatedBy,
		ApiCredentialID:  in.APICredentialID,
		Metadata:         metaBytes,
	})
	if err != nil {
		if existing, ok := s.existingJobOnUnique(ctx, q, in.ClientID, in.IdempotencyKey, err); ok {
			return CreateResult{Job: existing, Deduplicated: true, WorkUnits: int(existing.ItemCount)}, nil
		}
		return CreateResult{}, err
	}

	rows := make([]sqlcdb.InsertLookupItemsParams, 0, len(phones))
	cur := currency
	for _, phone := range phones {
		p := phone
		d := PhoneDigits(p)
		share := price
		rows = append(rows, sqlcdb.InsertLookupItemsParams{
			JobID:          job.ID,
			ClientID:       in.ClientID,
			CheckType:      in.CheckType,
			PhoneE164:      p,
			PhoneDigits:    d,
			UnitSellPrice:  &share,
			TariffPlanID:   &planID,
			TariffPlanCode: &planCode,
			Currency:       &cur,
			EstimatedCost:  &share,
		})
	}
	if _, err := q.InsertLookupItems(ctx, rows); err != nil {
		return CreateResult{}, err
	}
	if err := s.billing.ReserveForLookupJob(ctx, q, job); err != nil {
		return CreateResult{}, mapBillingErr(err)
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return CreateResult{}, err
		}
	}

	s.log.Info("lookup.create",
		"job_id", job.ID,
		"client_id", in.ClientID,
		"check_type", in.CheckType,
		"work_units", len(phones),
		"deduplicated_phones", deduped,
	)
	return CreateResult{
		Job:                    job,
		DeduplicatedPhoneCount: deduped,
		WorkUnits:              len(phones),
	}, nil
}

func (s *Service) FailJob(ctx context.Context, jobID uuid.UUID, code, msg string) (sqlcdb.LookupJob, error) {
	if s == nil || s.store == nil {
		return sqlcdb.LookupJob{}, errors.New("lookup service not configured")
	}
	finalized, err := s.store.Queries.FinalizeLookupJob(ctx, sqlcdb.FinalizeLookupJobParams{
		Status:       sqlcdb.LookupJobStatusFailed,
		ErrorCode:    strPtrOrNil(code),
		ErrorMessage: strPtrOrNil(msg),
		ID:           jobID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return s.store.Queries.GetLookupJob(ctx, jobID)
		}
		return sqlcdb.LookupJob{}, err
	}
	if s.hooks != nil {
		if _, err := s.hooks.EnqueueJob(ctx, finalized); err != nil && s.log != nil {
			s.log.Error("lookup webhook job", "job_id", finalized.ID, "err", err)
		}
	}
	return finalized, nil
}

func mapBillingErr(err error) error {
	var be *billing.Error
	if errors.As(err, &be) {
		switch be.Code {
		case "tariff_not_configured":
			return wrap(ErrTariffNotConfigured, be.Code, be.Message)
		default:
			return wrap(be.Err, be.Code, be.Message)
		}
	}
	return err
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == pgerrcode.UniqueViolation
}

func (s *Service) jobByIdempotency(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID, key string) (sqlcdb.LookupJob, error) {
	arg := sqlcdb.GetLookupJobByIdempotencyParams{
		ClientID:       clientID,
		IdempotencyKey: &key,
	}
	if q != nil {
		job, err := q.GetLookupJobByIdempotency(ctx, arg)
		if err == nil || !errors.Is(err, pgx.ErrNoRows) {
			return job, err
		}
	}
	if s.store != nil && s.store.Queries != nil && (q == nil || q != s.store.Queries) {
		return s.store.Queries.GetLookupJobByIdempotency(ctx, arg)
	}
	return sqlcdb.LookupJob{}, pgx.ErrNoRows
}

func (s *Service) existingJobOnUnique(ctx context.Context, q *sqlcdb.Queries, clientID uuid.UUID, key string, insertErr error) (sqlcdb.LookupJob, bool) {
	if !isUniqueViolation(insertErr) || key == "" {
		return sqlcdb.LookupJob{}, false
	}
	job, err := s.jobByIdempotency(ctx, q, clientID, key)
	if err != nil {
		return sqlcdb.LookupJob{}, false
	}
	return job, true
}

package lookup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

type Persistence struct {
	q *sqlcdb.Queries
}

func NewPersistence(q *sqlcdb.Queries) *Persistence {
	return &Persistence{q: q}
}

func (p *Persistence) SaveRequest(ctx context.Context, record smsc.RequestRecord) (smsc.SaveResult, error) {
	if p == nil || p.q == nil {
		return smsc.SaveResult{}, errors.New("lookup persistence not configured")
	}
	if tid := parseUUIDPtr(record.TenantID); tid != nil {
		if cl, err := p.q.GetClientByID(ctx, *tid); err == nil && cl.Status == sqlcdb.ClientStatusDeleted {
			return smsc.SaveResult{}, nil
		}
	}
	kind, err := mapRequestKind(record.Kind)
	if err != nil {
		return smsc.SaveResult{}, err
	}
	payload, _ := json.Marshal(record.RequestPayload)
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	row, err := p.q.InsertProviderLookupRequest(ctx, sqlcdb.InsertProviderLookupRequestParams{
		ClientID:          parseUUIDPtr(record.TenantID),
		JobItemID:         parseUUIDPtr(record.JobItemID),
		ProviderCode:      record.ProviderCode,
		Kind:              kind,
		Status:            mapRequestStatus(record.Status),
		ProviderMessageID: strPtr(record.ProviderMessageID),
		RequestPayload:    payload,
		IdempotencyKey:    strPtr(record.IdempotencyKey),
		StartedAt:         timePtr(record.StartedAt),
	})
	if err != nil {
		if isUniqueViolation(err) && record.IdempotencyKey != "" {
			existing, findErr := p.q.GetLatestProviderSendByIdempotency(ctx, sqlcdb.GetLatestProviderSendByIdempotencyParams{
				ProviderCode:   record.ProviderCode,
				IdempotencyKey: &record.IdempotencyKey,
			})
			if findErr == nil {
				if existing.Status == sqlcdb.ProviderLookupRequestStatusPending {
					return smsc.SaveResult{}, &smsc.IdempotencyConflictError{IdempotencyKey: record.IdempotencyKey}
				}
				if existing.Status == sqlcdb.ProviderLookupRequestStatusSucceeded {
					return smsc.SaveResult{ID: existing.ID.String(), Deduplicated: true}, nil
				}
			}
		}
		return smsc.SaveResult{}, err
	}
	return smsc.SaveResult{ID: row.ID.String(), Deduplicated: false}, nil
}

func (p *Persistence) UpdateRequest(ctx context.Context, id string, patch smsc.RequestPatch) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	var norm []byte
	if patch.Normalized != nil {
		norm = normalizedToJSON(*patch.Normalized)
	}
	var resp []byte
	if patch.ResponsePayload != nil {
		resp, _ = json.Marshal(patch.ResponsePayload)
	}
	httpStatus := int32(patch.HTTPStatus)
	var httpPtr *int32
	if patch.HTTPStatus != 0 {
		httpPtr = &httpStatus
	}
	return p.q.UpdateProviderLookupRequest(ctx, sqlcdb.UpdateProviderLookupRequestParams{
		Status:            mapRequestStatus(patch.Status),
		ProviderMessageID: strPtr(patch.ProviderMessageID),
		HttpStatus:        httpPtr,
		ErrorCode:         strPtr(patch.ErrorCode),
		ErrorMessage:      strPtr(patch.ErrorMessage),
		ResponsePayload:   resp,
		NormalizedResult:  norm,
		FinishedAt:        timePtr(patch.FinishedAt),
		ID:                uid,
	})
}

func (p *Persistence) FindSucceededSend(ctx context.Context, providerCode, tenantID, idempotencyKey string) (*smsc.RequestRecord, error) {
	row, err := p.q.GetSucceededProviderSendByIdempotency(ctx, sqlcdb.GetSucceededProviderSendByIdempotencyParams{
		ProviderCode:   providerCode,
		IdempotencyKey: &idempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec := requestFromRow(row)
	return &rec, nil
}

func (p *Persistence) FindLatestSend(ctx context.Context, providerCode, tenantID, idempotencyKey string) (*smsc.RequestRecord, error) {
	row, err := p.q.GetLatestProviderSendByIdempotency(ctx, sqlcdb.GetLatestProviderSendByIdempotencyParams{
		ProviderCode:   providerCode,
		IdempotencyKey: &idempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec := requestFromRow(row)
	return &rec, nil
}

func (p *Persistence) SaveCallback(ctx context.Context, record smsc.CallbackRecord) (smsc.SaveResult, error) {
	raw, _ := json.Marshal(record.RawPayload)
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var norm []byte
	if record.Normalized != nil {
		norm = normalizedToJSON(*record.Normalized)
	}
	row, err := p.q.InsertProviderLookupCallback(ctx, sqlcdb.InsertProviderLookupCallbackParams{
		ClientID:          parseUUIDPtr(record.TenantID),
		JobItemID:         parseUUIDPtr(record.JobItemID),
		ProviderCode:      record.ProviderCode,
		ProviderMessageID: strPtr(record.ProviderMessageID),
		RawPayload:        raw,
		NormalizedResult:  norm,
		DedupeKey:         strPtr(record.DedupeKey),
		SignatureValid:    record.SignatureValid,
		ProcessedAt:       timePtr(record.ProcessedAt),
		ProcessError:      strPtr(record.ProcessError),
	})
	if err != nil {
		if isUniqueViolation(err) && record.DedupeKey != "" {
			existing, findErr := p.q.GetProviderLookupCallbackByDedupe(ctx, sqlcdb.GetProviderLookupCallbackByDedupeParams{
				ProviderCode: record.ProviderCode,
				DedupeKey:    &record.DedupeKey,
			})
			if findErr == nil {
				return smsc.SaveResult{ID: existing.ID.String(), Deduplicated: true}, nil
			}
		}
		return smsc.SaveResult{}, err
	}
	return smsc.SaveResult{ID: row.ID.String(), Deduplicated: false}, nil
}

func requestFromRow(row sqlcdb.ProviderLookupRequest) smsc.RequestRecord {
	var req any
	_ = json.Unmarshal(row.RequestPayload, &req)
	var resp any
	if len(row.ResponsePayload) > 0 {
		_ = json.Unmarshal(row.ResponsePayload, &resp)
	}
	rec := smsc.RequestRecord{
		ID:                row.ID.String(),
		ProviderCode:      row.ProviderCode,
		Kind:              smsc.RequestKind(strings.ToUpper(string(row.Kind))),
		Status:            smsc.RequestStatus(strings.ToUpper(string(row.Status))),
		RequestPayload:    req,
		ResponsePayload:   resp,
		IdempotencyKey:    deref(row.IdempotencyKey),
		ProviderMessageID: deref(row.ProviderMessageID),
	}
	if row.ClientID != nil {
		rec.TenantID = row.ClientID.String()
	}
	if row.JobItemID != nil {
		rec.JobItemID = row.JobItemID.String()
	}
	if row.StartedAt != nil {
		rec.StartedAt = *row.StartedAt
	}
	if row.FinishedAt != nil {
		rec.FinishedAt = *row.FinishedAt
	}
	return rec
}

func mapRequestKind(k smsc.RequestKind) (sqlcdb.ProviderLookupKind, error) {
	switch strings.ToUpper(string(k)) {
	case "SEND":
		return sqlcdb.ProviderLookupKindSend, nil
	case "STATUS":
		return sqlcdb.ProviderLookupKindStatus, nil
	case "COST":
		return sqlcdb.ProviderLookupKindCost, nil
	case "BALANCE":
		return sqlcdb.ProviderLookupKindBalance, nil
	default:
		return "", wrap(ErrValidation, "validation", "unknown provider request kind")
	}
}

func mapRequestStatus(s smsc.RequestStatus) sqlcdb.ProviderLookupRequestStatus {
	switch strings.ToUpper(string(s)) {
	case "SUCCEEDED":
		return sqlcdb.ProviderLookupRequestStatusSucceeded
	case "FAILED":
		return sqlcdb.ProviderLookupRequestStatusFailed
	default:
		return sqlcdb.ProviderLookupRequestStatusPending
	}
}

func parseUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

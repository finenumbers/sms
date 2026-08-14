package lookup

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const previewTTL = 30 * time.Minute

type CSVPreviewInput struct {
	ClientID uuid.UUID
	Type     sqlcdb.LookupCheckType
	Filename string
	Body     []byte
}

func ParseCSVPhones(raw []byte) []string {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil
	}
	if strings.Contains(text, ",") || strings.Contains(text, ";") {
		r := csv.NewReader(bytes.NewReader(raw))
		r.FieldsPerRecord = -1
		r.LazyQuotes = true
		if strings.Contains(text, ";") && !strings.Contains(text, ",") {
			r.Comma = ';'
		}
		var out []string
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
			for _, cell := range rec {
				cell = strings.TrimSpace(cell)
				if cell == "" || strings.EqualFold(cell, "phone") || strings.EqualFold(cell, "телефон") {
					continue
				}
				out = append(out, cell)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.EqualFold(line, "phone") || strings.EqualFold(line, "телефон") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func (s *Service) CreateCSVPreview(ctx context.Context, in CSVPreviewInput) (sqlcdb.LookupCsvPreview, error) {
	view, err := s.settings.Get(ctx)
	if err != nil {
		return sqlcdb.LookupCsvPreview{}, err
	}
	if !view.LookupEnabled {
		return sqlcdb.LookupCsvPreview{}, wrap(ErrLookupDisabled, "lookup_disabled", "lookup is disabled")
	}
	if err := s.ensureClient(ctx, in.ClientID); err != nil {
		return sqlcdb.LookupCsvPreview{}, err
	}
	if in.Type != sqlcdb.LookupCheckTypeHlr && in.Type != sqlcdb.LookupCheckTypePing {
		return sqlcdb.LookupCsvPreview{}, wrap(ErrValidation, "validation", "type must be hlr or ping")
	}
	if int32(len(in.Body)) > view.LookupMaxCSVBytes {
		return sqlcdb.LookupCsvPreview{}, wrap(ErrValidation, "validation", "CSV exceeds max_csv_bytes")
	}
	if !utf8.Valid(in.Body) {
		return sqlcdb.LookupCsvPreview{}, wrap(ErrValidation, "validation", "CSV must be UTF-8")
	}
	rawPhones := ParseCSVPhones(in.Body)
	phones, deduped, err := PreparePhones(rawPhones, "bulk", int(view.LookupMaxCSVRows), "max_csv_rows")
	if err != nil {
		return sqlcdb.LookupCsvPreview{}, err
	}
	payload, _ := json.Marshal(map[string]any{
		"phones":          phones,
		"row_count":       len(rawPhones),
		"invalid_count":   0,
		"duplicate_count": deduped,
	})
	name := strings.TrimSpace(in.Filename)
	var fname *string
	if name != "" {
		fname = &name
	}
	return s.store.Queries.InsertLookupCSVPreview(ctx, sqlcdb.InsertLookupCSVPreviewParams{
		ClientID:         in.ClientID,
		CheckType:        in.Type,
		Status:           sqlcdb.LookupCsvPreviewStatusReady,
		OriginalFilename: fname,
		PhoneCount:       int32(len(phones)),
		PhonesJson:       payload,
		ExpiresAt:        time.Now().UTC().Add(previewTTL),
	})
}

func (s *Service) GetCSVPreview(ctx context.Context, clientID, id uuid.UUID) (sqlcdb.LookupCsvPreview, []string, error) {
	row, err := s.store.Queries.GetLookupCSVPreviewForClient(ctx, sqlcdb.GetLookupCSVPreviewForClientParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		if errorsIsNoRows(err) {
			return sqlcdb.LookupCsvPreview{}, nil, wrap(ErrNotFound, "not_found", "preview not found")
		}
		return sqlcdb.LookupCsvPreview{}, nil, err
	}
	if time.Now().UTC().After(row.ExpiresAt) {
		return sqlcdb.LookupCsvPreview{}, nil, wrap(ErrNotFound, "not_found", "preview expired")
	}
	phones, err := phonesFromPreview(row)
	if err != nil {
		return row, nil, err
	}
	return row, phones, nil
}

func (s *Service) SubmitCSVPreview(ctx context.Context, clientID, previewID uuid.UUID, createdBy *uuid.UUID) (CreateResult, error) {
	row, phones, err := s.GetCSVPreview(ctx, clientID, previewID)
	if err != nil {
		return CreateResult{}, err
	}
	if row.Status == sqlcdb.LookupCsvPreviewStatusConsumed && row.JobID != nil {
		return s.resultFromExistingJob(ctx, clientID, *row.JobID)
	}
	if row.Status == sqlcdb.LookupCsvPreviewStatusConsuming {
		return CreateResult{}, wrap(ErrConflict, "conflict", "preview is being submitted")
	}
	if row.Status != sqlcdb.LookupCsvPreviewStatusReady {
		return CreateResult{}, wrap(ErrConflict, "conflict", "preview is not ready")
	}

	claimed, err := s.store.Queries.ClaimLookupCSVPreviewReady(ctx, sqlcdb.ClaimLookupCSVPreviewReadyParams{
		ID:       row.ID,
		ClientID: clientID,
	})
	if err != nil {
		if errorsIsNoRows(err) {
			fresh, _, getErr := s.GetCSVPreview(ctx, clientID, previewID)
			if getErr != nil {
				return CreateResult{}, getErr
			}
			if fresh.Status == sqlcdb.LookupCsvPreviewStatusConsumed && fresh.JobID != nil {
				return s.resultFromExistingJob(ctx, clientID, *fresh.JobID)
			}
			if fresh.Status == sqlcdb.LookupCsvPreviewStatusConsuming {
				return CreateResult{}, wrap(ErrConflict, "conflict", "preview is being submitted")
			}
			return CreateResult{}, wrap(ErrConflict, "conflict", "preview is not ready")
		}
		return CreateResult{}, err
	}

	if existing, findErr := s.store.Queries.GetLookupJobByCSVPreview(ctx, sqlcdb.GetLookupJobByCSVPreviewParams{
		ClientID:  clientID,
		PreviewID: claimed.ID.String(),
	}); findErr == nil {
		jobID := existing.ID
		_, _ = s.store.Queries.MarkLookupCSVPreviewConsumed(ctx, sqlcdb.MarkLookupCSVPreviewConsumedParams{
			JobID: &jobID,
			ID:    claimed.ID,
		})
		return CreateResult{Job: existing, Deduplicated: true, WorkUnits: int(existing.ItemCount)}, nil
	}

	view, err := s.settings.Get(ctx)
	if err != nil {
		s.rollbackPreview(ctx, claimed.ID)
		return CreateResult{}, err
	}
	out, err := s.Create(ctx, CreateInput{
		ClientID:         clientID,
		CheckType:        claimed.CheckType,
		Source:           sqlcdb.LookupJobSourceBulk,
		Phones:           phones,
		CreatedBy:        createdBy,
		OriginalFilename: claimed.OriginalFilename,
		MaxPhones:        int(view.LookupMaxCSVRows),
		PhoneCapName:     "max_csv_rows",
		Metadata:         map[string]any{"csv_preview_id": claimed.ID.String()},
	})
	if err != nil {
		s.rollbackPreview(ctx, claimed.ID)
		return CreateResult{}, err
	}
	jobID := out.Job.ID
	if _, markErr := s.store.Queries.MarkLookupCSVPreviewConsumed(ctx, sqlcdb.MarkLookupCSVPreviewConsumedParams{
		JobID: &jobID,
		ID:    claimed.ID,
	}); markErr != nil && s.log != nil {
		s.log.Error("lookup csv preview consume", "preview_id", claimed.ID, "job_id", jobID, "err", markErr)
	}
	return out, nil
}

func (s *Service) resultFromExistingJob(ctx context.Context, clientID, jobID uuid.UUID) (CreateResult, error) {
	job, err := s.GetJobForClient(ctx, clientID, jobID)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Job: job, Deduplicated: true, WorkUnits: int(job.ItemCount)}, nil
}

func (s *Service) CreateCSVShell(ctx context.Context, qin *sqlcdb.Queries, in CreateInput, raw []byte, filename string) (sqlcdb.LookupJob, error) {
	if err := s.ensureClient(ctx, in.ClientID); err != nil {
		return sqlcdb.LookupJob{}, err
	}
	view, err := s.settings.Get(ctx)
	if err != nil {
		return sqlcdb.LookupJob{}, err
	}
	if !view.LookupEnabled {
		return sqlcdb.LookupJob{}, wrap(ErrLookupDisabled, "lookup_disabled", "lookup is disabled")
	}
	if int32(len(raw)) > view.LookupMaxCSVBytes {
		return sqlcdb.LookupJob{}, wrap(ErrValidation, "validation", "CSV exceeds max_csv_bytes")
	}
	if err := s.billing.AssertLookupAssignment(ctx, nil, in.ClientID, in.CheckType); err != nil {
		return sqlcdb.LookupJob{}, mapBillingErr(err)
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
			return sqlcdb.LookupJob{}, err
		}
		defer begun.Rollback(ctx)
		if err := SetCreateStatementTimeout(ctx, begun); err != nil {
			return sqlcdb.LookupJob{}, err
		}
		q = s.store.Queries.WithTx(begun)
		tx = begun
	}

	meta, _ := json.Marshal(map[string]any{"csv_file_path": filename})
	job, err := q.InsertLookupJob(ctx, sqlcdb.InsertLookupJobParams{
		ClientID:         in.ClientID,
		CheckType:        in.CheckType,
		Source:           sqlcdb.LookupJobSourceApi,
		Status:           sqlcdb.LookupJobStatusQueued,
		ItemCount:        0,
		Currency:         "RUB",
		OriginalFilename: strPtrOrNil(filename),
		IdempotencyKey:   strPtrOrNil(in.IdempotencyKey),
		CreatedBy:        in.CreatedBy,
		ApiCredentialID:  in.APICredentialID,
		Metadata:         meta,
	})
	if err != nil {
		if existing, ok := s.existingJobOnUnique(ctx, q, in.ClientID, in.IdempotencyKey, err); ok {
			return existing, nil
		}
		return sqlcdb.LookupJob{}, err
	}
	payload, _ := json.Marshal(map[string]any{"raw": string(raw)})
	jobID := job.ID
	_, err = q.InsertLookupCSVPreview(ctx, sqlcdb.InsertLookupCSVPreviewParams{
		ClientID:         in.ClientID,
		CheckType:        in.CheckType,
		Status:           sqlcdb.LookupCsvPreviewStatusReady,
		OriginalFilename: strPtrOrNil(filename),
		PhoneCount:       0,
		PhonesJson:       payload,
		JobID:            &jobID,
		ExpiresAt:        time.Now().UTC().Add(24 * time.Hour),
	})
	if err != nil {
		return sqlcdb.LookupJob{}, err
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return sqlcdb.LookupJob{}, err
		}
	}
	return job, nil
}

func (s *Service) MaterializeCSVJob(ctx context.Context, preview sqlcdb.LookupCsvPreview) error {
	if preview.JobID == nil {
		return nil
	}
	raw, _ := rawFromPreview(preview)
	view, err := s.settings.Get(ctx)
	if err != nil {
		return err
	}
	phones, _, err := PreparePhones(ParseCSVPhones([]byte(raw)), "bulk", int(view.LookupMaxCSVRows), "max_csv_rows")
	if err != nil {
		msg := err.Error()
		if le := AsError(err); le != nil {
			msg = le.Message
		}
		_, _ = s.FailJob(ctx, *preview.JobID, "csv_parse_failed", msg)
		s.invalidatePreview(ctx, preview.ID, msg)
		return nil
	}
	est, err := s.billing.EstimateLookup(ctx, preview.ClientID, preview.CheckType, int64(len(phones)))
	if err != nil {
		msg := err.Error()
		code := "tariff_not_configured"
		if httpxCode := billingCode(err); httpxCode != "" {
			code = httpxCode
		}
		_, _ = s.FailJob(ctx, *preview.JobID, code, msg)
		s.invalidatePreview(ctx, preview.ID, msg)
		return nil
	}
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := SetCreateStatementTimeout(ctx, tx); err != nil {
		return err
	}
	q := s.store.Queries.WithTx(tx)
	price := est.UnitSellPrice
	planID := est.TariffPlanID
	planCode := est.TariffPlanCode
	total := est.Total
	currency := est.Currency
	if currency == "" {
		currency = "RUB"
	}
	job, err := q.PatchLookupJobAfterParse(ctx, sqlcdb.PatchLookupJobAfterParseParams{
		ItemCount:      int32(len(phones)),
		UnitSellPrice:  &price,
		TariffPlanID:   &planID,
		TariffPlanCode: &planCode,
		Currency:       currency,
		EstimatedCost:  &total,
		ID:             *preview.JobID,
	})
	if err != nil {
		if errorsIsNoRows(err) {
			consumed := sqlcdb.LookupCsvPreviewStatusConsumed
			_, _ = s.store.Queries.UpdateLookupCSVPreview(ctx, sqlcdb.UpdateLookupCSVPreviewParams{
				Status: nullPreviewStatus(&consumed),
				ID:     preview.ID,
			})
			return nil
		}
		return err
	}
	rows := make([]sqlcdb.InsertLookupItemsParams, 0, len(phones))
	cur := currency
	for _, phone := range phones {
		p := phone
		share := price
		rows = append(rows, sqlcdb.InsertLookupItemsParams{
			JobID:          job.ID,
			ClientID:       job.ClientID,
			CheckType:      job.CheckType,
			PhoneE164:      p,
			PhoneDigits:    PhoneDigits(p),
			UnitSellPrice:  &share,
			TariffPlanID:   &planID,
			TariffPlanCode: &planCode,
			Currency:       &cur,
			EstimatedCost:  &share,
		})
	}
	if _, err := q.InsertLookupItems(ctx, rows); err != nil {
		return err
	}
	if err := stampClientSendIDs(ctx, q, job.ID); err != nil {
		return err
	}
	if err := s.billing.ReserveForLookupJob(ctx, q, job); err != nil {
		msg := err.Error()
		code := billingCode(err)
		if code == "" {
			code = "reserve_failed"
		}
		_ = tx.Rollback(ctx)
		_, _ = s.FailJob(ctx, job.ID, code, msg)
		s.invalidatePreview(ctx, preview.ID, msg)
		return nil
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	consumed := sqlcdb.LookupCsvPreviewStatusConsumed
	n := int32(len(phones))
	payload, _ := json.Marshal(map[string]any{"phones": phones})
	_, _ = s.store.Queries.UpdateLookupCSVPreview(ctx, sqlcdb.UpdateLookupCSVPreviewParams{
		Status:     nullPreviewStatus(&consumed),
		PhoneCount: &n,
		PhonesJson: payload,
		ID:         preview.ID,
	})
	return nil
}

func (s *Service) ensureClient(ctx context.Context, clientID uuid.UUID) error {
	client, err := s.store.Queries.GetClientByID(ctx, clientID)
	if err != nil {
		if errorsIsNoRows(err) {
			return wrap(ErrNotFound, "not_found", "client not found")
		}
		return err
	}
	if client.Status == sqlcdb.ClientStatusSuspended {
		return wrap(ErrClientSuspended, "client_suspended", "client is suspended")
	}
	if client.Status != sqlcdb.ClientStatusActive {
		return wrap(ErrNotFound, "not_found", "client not found")
	}
	return nil
}

func (s *Service) rollbackPreview(ctx context.Context, id uuid.UUID) {
	_, _ = s.store.Queries.RollbackLookupCSVPreviewConsuming(ctx, id)
}

func (s *Service) invalidatePreview(ctx context.Context, id uuid.UUID, msg string) {
	invalid := sqlcdb.LookupCsvPreviewStatusInvalid
	_, _ = s.store.Queries.UpdateLookupCSVPreview(ctx, sqlcdb.UpdateLookupCSVPreviewParams{
		Status:       nullPreviewStatus(&invalid),
		ErrorMessage: &msg,
		ID:           id,
	})
}

func (s *Service) ListCSVPreviewPhones(ctx context.Context, clientID, id uuid.UUID, limit, offset int32) ([]map[string]any, int, error) {
	_, phones, err := s.GetCSVPreview(ctx, clientID, id)
	if err != nil {
		return nil, 0, err
	}
	items, total := pagePreviewPhones(phones, limit, offset)
	return items, total, nil
}

func pagePreviewPhones(phones []string, limit, offset int32) ([]map[string]any, int) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	total := len(phones)
	start := int(offset)
	if start > total {
		start = total
	}
	end := start + int(limit)
	if end > total {
		end = total
	}
	out := make([]map[string]any, 0, end-start)
	for i, phone := range phones[start:end] {
		out = append(out, map[string]any{
			"phone": phone,
			"line":  start + i + 1,
		})
	}
	return out, total
}

func phonesFromPreview(row sqlcdb.LookupCsvPreview) ([]string, error) {
	var payload struct {
		Phones []string `json:"phones"`
		Raw    string   `json:"raw"`
	}
	if err := json.Unmarshal(row.PhonesJson, &payload); err != nil {
		return nil, wrap(ErrValidation, "validation", "preview payload is invalid")
	}
	if len(payload.Phones) > 0 {
		return payload.Phones, nil
	}
	return ParseCSVPhones([]byte(payload.Raw)), nil
}

func rawFromPreview(row sqlcdb.LookupCsvPreview) (string, error) {
	var payload struct {
		Raw string `json:"raw"`
	}
	if err := json.Unmarshal(row.PhonesJson, &payload); err != nil {
		return "", err
	}
	return payload.Raw, nil
}

func strPtrOrNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func billingCode(err error) string {
	if e := mapBillingErr(err); e != nil {
		if le := AsError(e); le != nil {
			return le.Code
		}
	}
	return ""
}

func (s *Service) DeleteCSVPreview(ctx context.Context, clientID, id uuid.UUID) error {
	n, err := s.store.Queries.DeleteLookupCSVPreview(ctx, sqlcdb.DeleteLookupCSVPreviewParams{
		ID:       id,
		ClientID: clientID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return wrap(ErrNotFound, "not_found", "preview not found")
	}
	return nil
}

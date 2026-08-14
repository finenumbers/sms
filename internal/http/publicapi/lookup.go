package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finenumbers/sms/internal/authctx"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/idempotency"
	"finenumbers/sms/internal/lookup"
)

const maxLookupJSON = 1 << 20
const maxLookupCSV = 12 << 20

type publicCheckRequest struct {
	Phone string `json:"phone"`
	Type  string `json:"type"`
}

type publicJobRequest struct {
	Type   string   `json:"type"`
	Phones []string `json:"phones"`
}

func (h *Handlers) requireLookup(w http.ResponseWriter, r *http.Request) (uuid.UUID, *uuid.UUID, bool) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.ClientID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return uuid.UUID{}, nil, false
	}
	if h.Lookup == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "lookup unavailable")
		return uuid.UUID{}, nil, false
	}
	return *p.ClientID, p.APIKeyID, true
}

func (h *Handlers) CreateCheck(w http.ResponseWriter, r *http.Request) {
	clientID, keyID, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	raw, req, ok := decodePublicJSON[publicCheckRequest](w, r, maxLookupJSON)
	if !ok {
		return
	}
	ct, err := lookup.ParseCheckType(req.Type)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	h.withLookupIdempotency(w, r, keyID, raw, func(ctx context.Context, q *sqlcdb.Queries) (int, any, error) {
		in := lookup.CreateInput{
			ClientID:        clientID,
			CheckType:       ct,
			Source:          sqlcdb.LookupJobSourceApi,
			Phones:          []string{req.Phone},
			IdempotencyKey:  strings.TrimSpace(r.Header.Get("Idempotency-Key")),
			APICredentialID: keyID,
		}
		var out lookup.CreateResult
		var err error
		if q != nil {
			out, err = h.Lookup.CreateWith(ctx, q, in)
		} else {
			out, err = h.Lookup.Create(ctx, in)
		}
		if err != nil {
			return 0, nil, err
		}
		return http.StatusAccepted, lookup.JobAcceptedJSON(out), nil
	})
}

func (h *Handlers) ListChecks(w http.ResponseWriter, r *http.Request) {
	clientID, _, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	var status *sqlcdb.LookupItemStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		st, err := lookup.ParseItemStatus(raw)
		if err != nil {
			lookup.WriteError(w, err)
			return
		}
		status = &st
	}
	var checkType *sqlcdb.LookupCheckType
	if raw := strings.TrimSpace(r.URL.Query().Get("check_type")); raw != "" {
		ct, err := lookup.ParseCheckType(raw)
		if err != nil {
			lookup.WriteError(w, err)
			return
		}
		checkType = &ct
	}
	rows, total, err := h.Lookup.ListItemsForClient(r.Context(), clientID, status, checkType, limit, offset)
	if err != nil {
		h.Log.Error("list lookup checks", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, it := range rows {
		items = append(items, lookup.CheckJSON(it))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handlers) GetCheck(w http.ResponseWriter, r *http.Request) {
	clientID, _, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "checkID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	item, job, err := h.Lookup.GetCheckOrJob(r.Context(), clientID, id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	if item != nil {
		httpx.WriteJSON(w, http.StatusOK, lookup.CheckJSON(*item))
		return
	}
	body := lookup.JobJSON(*job)
	body["kind"] = "job"
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handlers) CreateJob(w http.ResponseWriter, r *http.Request) {
	clientID, keyID, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	raw, req, ok := decodePublicJSON[publicJobRequest](w, r, maxLookupJSON)
	if !ok {
		return
	}
	ct, err := lookup.ParseCheckType(req.Type)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	h.withLookupIdempotency(w, r, keyID, raw, func(ctx context.Context, q *sqlcdb.Queries) (int, any, error) {
		in := lookup.CreateInput{
			ClientID:        clientID,
			CheckType:       ct,
			Source:          sqlcdb.LookupJobSourceApi,
			Phones:          req.Phones,
			IdempotencyKey:  strings.TrimSpace(r.Header.Get("Idempotency-Key")),
			APICredentialID: keyID,
		}
		var out lookup.CreateResult
		var err error
		if q != nil {
			out, err = h.Lookup.CreateWith(ctx, q, in)
		} else {
			out, err = h.Lookup.Create(ctx, in)
		}
		if err != nil {
			return 0, nil, err
		}
		return http.StatusAccepted, lookup.JobAcceptedJSON(out), nil
	})
}

func (h *Handlers) CreateJobCSV(w http.ResponseWriter, r *http.Request) {
	clientID, keyID, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLookupCSV)
	ctRaw := r.URL.Query().Get("type")
	filename := "upload.csv"
	var raw []byte
	if strings.Contains(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(maxLookupCSV); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "file required")
			return
		}
		if v := r.FormValue("type"); v != "" {
			ctRaw = v
		}
		file, hdr, err := r.FormFile("file")
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "file field required")
			return
		}
		defer file.Close()
		raw, err = io.ReadAll(io.LimitReader(file, maxLookupCSV+1))
		if err != nil || len(raw) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "cannot read file")
			return
		}
		if hdr != nil && hdr.Filename != "" {
			filename = hdr.Filename
		}
	} else {
		var err error
		raw, err = io.ReadAll(io.LimitReader(r.Body, maxLookupCSV+1))
		if err != nil || len(raw) == 0 {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "CSV body required")
			return
		}
	}
	ct, err := lookup.ParseCheckType(ctRaw)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	hashBody := append([]byte(ctRaw+"\n"), raw...)
	h.withLookupIdempotency(w, r, keyID, hashBody, func(ctx context.Context, q *sqlcdb.Queries) (int, any, error) {
		job, err := h.Lookup.CreateCSVShell(ctx, q, lookup.CreateInput{
			ClientID:         clientID,
			CheckType:        ct,
			Source:           sqlcdb.LookupJobSourceApi,
			IdempotencyKey:   strings.TrimSpace(r.Header.Get("Idempotency-Key")),
			APICredentialID:  keyID,
			OriginalFilename: &filename,
		}, raw, filename)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusAccepted, lookup.JobJSON(job), nil
	})
}

func (h *Handlers) ListJobs(w http.ResponseWriter, r *http.Request) {
	clientID, _, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	f := lookup.ListJobsFilter{ClientID: &clientID, Limit: limit, Offset: offset}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		st, err := lookup.ParseJobStatus(raw)
		if err != nil {
			lookup.WriteError(w, err)
			return
		}
		f.Status = &st
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("check_type")); raw != "" {
		ct, err := lookup.ParseCheckType(raw)
		if err != nil {
			lookup.WriteError(w, err)
			return
		}
		f.CheckType = &ct
	}
	rows, total, err := h.Lookup.ListJobs(r.Context(), f)
	if err != nil {
		h.Log.Error("list lookup jobs", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, job := range rows {
		items = append(items, lookup.JobJSON(job))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handlers) GetJob(w http.ResponseWriter, r *http.Request) {
	clientID, _, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	job, err := h.Lookup.GetJobForClient(r.Context(), clientID, id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.JobJSON(job))
}

func (h *Handlers) ListJobItems(w http.ResponseWriter, r *http.Request) {
	clientID, _, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "jobID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	if _, err := h.Lookup.GetJobForClient(r.Context(), clientID, id); err != nil {
		lookup.WriteError(w, err)
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	rows, total, err := h.Lookup.ListItems(r.Context(), id, limit, offset)
	if err != nil {
		h.Log.Error("list lookup job items", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, it := range rows {
		items = append(items, lookup.ItemJSON(it))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handlers) withLookupIdempotency(w http.ResponseWriter, r *http.Request, keyID *uuid.UUID, raw []byte, exec func(context.Context, *sqlcdb.Queries) (int, any, error)) {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || keyID == nil || h.Store == nil {
		status, body, err := exec(r.Context(), nil)
		if err != nil {
			lookup.WriteError(w, err)
			return
		}
		httpx.WriteJSON(w, status, body)
		return
	}
	if len(key) > maxIdempotencyKey {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "Idempotency-Key too long")
		return
	}
	tx, err := h.Store.Pool.Begin(r.Context())
	if err != nil {
		h.Log.Error("lookup idempotency begin", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	defer tx.Rollback(r.Context())
	if err := lookup.SetCreateStatementTimeout(r.Context(), tx); err != nil {
		h.Log.Error("lookup idempotency timeout", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	q := h.Store.Queries.WithTx(tx)
	rec, err := idempotency.Reserve(r.Context(), q, sqlcdb.ActorTypeApiKey, *keyID, key, idempotency.HashRequest(r.Method, r.URL.Path, raw), idempotency.DefaultTTL)
	if err != nil {
		switch {
		case errors.Is(err, idempotency.ErrConflict):
			httpx.WriteError(w, http.StatusConflict, "idempotency_conflict", "Idempotency-Key reused with a different request")
		case errors.Is(err, idempotency.ErrInFlight):
			httpx.WriteError(w, http.StatusConflict, "idempotency_in_flight", "request with this Idempotency-Key is in progress")
		default:
			h.Log.Error("lookup idempotency reserve", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		}
		return
	}
	if rec.Replay {
		if err := tx.Commit(r.Context()); err != nil {
			h.Log.Error("lookup idempotency replay commit", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.Status)
		_, _ = w.Write(rec.Body)
		if len(rec.Body) == 0 || rec.Body[len(rec.Body)-1] != '\n' {
			_, _ = w.Write([]byte("\n"))
		}
		return
	}
	status, body, err := exec(r.Context(), q)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	payload, err := json.Marshal(body)
	if err != nil {
		h.Log.Error("lookup idempotency marshal", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err := idempotency.Complete(r.Context(), q, rec.ID, status, payload); err != nil {
		h.Log.Error("lookup idempotency complete", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.Log.Error("lookup idempotency commit", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, status, body)
}

func decodePublicJSON[T any](w http.ResponseWriter, r *http.Request, max int) ([]byte, T, bool) {
	var zero T
	raw, err := io.ReadAll(io.LimitReader(r.Body, int64(max)+1))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return nil, zero, false
	}
	if len(raw) > max {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "request body too large")
		return nil, zero, false
	}
	var req T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return nil, zero, false
	}
	return raw, req, true
}

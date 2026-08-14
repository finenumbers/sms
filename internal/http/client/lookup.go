package client

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/lookup"
)

const maxLookupUpload = 12 << 20

func (h *Handlers) requireLookup(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := requireClient(w, r)
	if !ok {
		return identity.Principal{}, false
	}
	if h.Lookup == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "lookup unavailable")
		return identity.Principal{}, false
	}
	return p, true
}

type lookupEstimateRequest struct {
	Type   string   `json:"type"`
	Phone  string   `json:"phone"`
	Phones []string `json:"phones"`
}

type lookupCheckRequest struct {
	Type  string `json:"type"`
	Phone string `json:"phone"`
}

type lookupJobRequest struct {
	Type   string   `json:"type"`
	Phones []string `json:"phones"`
}

func (h *Handlers) LookupEstimate(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	var req lookupEstimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	ct, err := lookup.ParseCheckType(req.Type)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	phones := req.Phones
	if req.Phone != "" {
		phones = append(phones, req.Phone)
	}
	est, n, err := h.Lookup.Estimate(r.Context(), *p.ClientID, ct, phones, 0, "")
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.EstimateJSON(est, n))
}

func (h *Handlers) LookupCreateCheck(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	var req lookupCheckRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	ct, err := lookup.ParseCheckType(req.Type)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	out, err := h.Lookup.Create(r.Context(), lookup.CreateInput{
		ClientID:  *p.ClientID,
		CheckType: ct,
		Source:    sqlcdb.LookupJobSourceSingle,
		Phones:    []string{req.Phone},
		CreatedBy: p.ClientUserID,
	})
	if err != nil {
		h.Log.Error("create lookup check", "err", err)
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, lookup.JobAcceptedJSON(out))
}

func (h *Handlers) LookupCreateJob(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	var req lookupJobRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	ct, err := lookup.ParseCheckType(req.Type)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	out, err := h.Lookup.Create(r.Context(), lookup.CreateInput{
		ClientID:  *p.ClientID,
		CheckType: ct,
		Source:    sqlcdb.LookupJobSourceBulk,
		Phones:    req.Phones,
		CreatedBy: p.ClientUserID,
	})
	if err != nil {
		h.Log.Error("create lookup job", "err", err)
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, lookup.JobAcceptedJSON(out))
}

func (h *Handlers) LookupListJobs(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	f, err := lookupListFilter(r, p.ClientID)
	if err != nil {
		lookup.WriteError(w, err)
		return
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

func (h *Handlers) LookupGetJob(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "jobID")
	if !ok {
		return
	}
	job, err := h.Lookup.GetJobForClient(r.Context(), *p.ClientID, id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.JobJSON(job))
}

func (h *Handlers) LookupListItems(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "jobID")
	if !ok {
		return
	}
	if _, err := h.Lookup.GetJobForClient(r.Context(), *p.ClientID, id); err != nil {
		lookup.WriteError(w, err)
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	rows, total, err := h.Lookup.ListItems(r.Context(), id, limit, offset)
	if err != nil {
		h.Log.Error("list lookup items", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, it := range rows {
		items = append(items, lookup.ItemJSON(it))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handlers) LookupExportJob(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "jobID")
	if !ok {
		return
	}
	job, err := h.Lookup.GetJobForClient(r.Context(), *p.ClientID, id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	raw, err := h.Lookup.ExportJobItems(r.Context(), job)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="lookup-%s-%s.xlsx"`, job.CheckType, job.ID))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(raw)
}

func (h *Handlers) LookupCreateCSVPreview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxLookupUpload)
	if err := r.ParseMultipartForm(maxLookupUpload); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "file required")
		return
	}
	ct, err := lookup.ParseCheckType(r.FormValue("type"))
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "file field required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxLookupUpload+1))
	if err != nil || len(raw) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "cannot read file")
		return
	}
	name := ""
	if hdr != nil {
		name = hdr.Filename
	}
	row, err := h.Lookup.CreateCSVPreview(r.Context(), lookup.CSVPreviewInput{
		ClientID: *p.ClientID,
		Type:     ct,
		Filename: name,
		Body:     raw,
	})
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, lookup.PreviewJSON(row))
}

func (h *Handlers) LookupGetCSVPreview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "previewID")
	if !ok {
		return
	}
	row, _, err := h.Lookup.GetCSVPreview(r.Context(), *p.ClientID, id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.PreviewJSON(row))
}

func (h *Handlers) LookupEstimateCSVPreview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "previewID")
	if !ok {
		return
	}
	row, phones, err := h.Lookup.GetCSVPreview(r.Context(), *p.ClientID, id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	view, err := h.Settings.Get(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	est, n, err := h.Lookup.Estimate(r.Context(), *p.ClientID, row.CheckType, phones, int(view.LookupMaxCSVRows), "max_csv_rows")
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.EstimateJSON(est, n))
}

func (h *Handlers) LookupSubmitCSVPreview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "previewID")
	if !ok {
		return
	}
	out, err := h.Lookup.SubmitCSVPreview(r.Context(), *p.ClientID, id, p.ClientUserID)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, lookup.JobAcceptedJSON(out))
}

func (h *Handlers) LookupDeleteCSVPreview(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireLookup(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "previewID")
	if !ok {
		return
	}
	if err := h.Lookup.DeleteCSVPreview(r.Context(), *p.ClientID, id); err != nil {
		lookup.WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func lookupListFilter(r *http.Request, clientID *uuid.UUID) (lookup.ListJobsFilter, error) {
	limit, offset := lookup.PageFromRequest(r)
	f := lookup.ListJobsFilter{ClientID: clientID, Limit: limit, Offset: offset}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		st, err := lookup.ParseJobStatus(raw)
		if err != nil {
			return f, err
		}
		f.Status = &st
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("check_type")); raw != "" {
		ct, err := lookup.ParseCheckType(raw)
		if err != nil {
			return f, err
		}
		f.CheckType = &ct
	}
	return f, nil
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

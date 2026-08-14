package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/authctx"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/smsc"
)

func (h *Handlers) requireLookup(w http.ResponseWriter) bool {
	if h.Lookup == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "lookup unavailable")
		return false
	}
	return true
}

func (h *Handlers) LookupListJobs(w http.ResponseWriter, r *http.Request) {
	if !h.requireLookup(w) {
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	f := lookup.ListJobsFilter{Limit: limit, Offset: offset}
	if raw := strings.TrimSpace(r.URL.Query().Get("client_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid client_id")
			return
		}
		f.ClientID = &id
	}
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
		h.Log.Error("admin list lookup jobs", "err", err)
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
	if !h.requireLookup(w) {
		return
	}
	id, ok := adminPathUUID(w, r, "jobID")
	if !ok {
		return
	}
	job, err := h.Lookup.GetJob(r.Context(), id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.JobJSON(job))
}

func (h *Handlers) LookupListItems(w http.ResponseWriter, r *http.Request) {
	if !h.requireLookup(w) {
		return
	}
	id, ok := adminPathUUID(w, r, "jobID")
	if !ok {
		return
	}
	if _, err := h.Lookup.GetJob(r.Context(), id); err != nil {
		lookup.WriteError(w, err)
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	rows, total, err := h.Lookup.ListItems(r.Context(), id, limit, offset)
	if err != nil {
		h.Log.Error("admin list lookup items", "err", err)
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
	if !h.requireLookup(w) {
		return
	}
	id, ok := adminPathUUID(w, r, "jobID")
	if !ok {
		return
	}
	job, err := h.Lookup.GetJob(r.Context(), id)
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

func (h *Handlers) LookupFinalizeJob(w http.ResponseWriter, r *http.Request) {
	if !h.requireLookup(w) {
		return
	}
	if h.LookupWorker == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "lookup worker unavailable")
		return
	}
	id, ok := adminPathUUID(w, r, "jobID")
	if !ok {
		return
	}
	if _, err := h.Lookup.GetJob(r.Context(), id); err != nil {
		lookup.WriteError(w, err)
		return
	}
	job, err := h.LookupWorker.FinalizeJob(r.Context(), id)
	if err != nil {
		lookup.WriteError(w, err)
		return
	}
	if p, ok := authctx.Principal(r.Context()); ok {
		h.Audit.Write(r.Context(), audit.Event{
			ActorType:    sqlcdb.ActorTypeAdmin,
			ActorID:      p.AdminUserID,
			Action:       "lookup.job.finalize",
			ResourceType: "lookup_job",
			ResourceID:   &job.ID,
			IP:           httpx.ClientIP(r),
			UserAgent:    httpx.UserAgent(r),
			Metadata:     map[string]any{"status": string(job.Status)},
		})
	}
	httpx.WriteJSON(w, http.StatusOK, lookup.JobJSON(job))
}

func (h *Handlers) LookupMonitoring(w http.ResponseWriter, r *http.Request) {
	if !h.requireLookup(w) {
		return
	}
	body, err := h.Lookup.Monitoring(r.Context(), h.SMSC)
	if err != nil {
		h.Log.Error("lookup monitoring", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

type smscEstimateRequest struct {
	Type  string `json:"type"`
	Phone string `json:"phone"`
}

func (h *Handlers) SMSCEstimateCost(w http.ResponseWriter, r *http.Request) {
	if h.SMSC == nil || !h.SMSC.Configured() {
		httpx.WriteError(w, http.StatusConflict, "not_configured", "smsc credentials are not configured")
		return
	}
	var req smscEstimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	ct := smsc.CheckHLR
	if strings.TrimSpace(req.Type) != "" {
		parsed, err := lookup.ParseCheckType(req.Type)
		if err != nil {
			lookup.WriteError(w, err)
			return
		}
		ct = smsc.CheckType(parsed)
	}
	est, err := h.SMSC.ProbeEstimateCost(r.Context(), ct, req.Phone)
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "smsc_error", err.Error())
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"provider_code": est.ProviderCode,
		"type":          est.CheckType,
		"phone":         est.PhoneE164,
		"cost":          est.Cost,
		"currency":      est.Currency,
		"parts":         est.Parts,
	})
}

func (h *Handlers) SMSCBalance(w http.ResponseWriter, r *http.Request) {
	if h.SMSC == nil || !h.SMSC.Configured() {
		httpx.WriteError(w, http.StatusConflict, "not_configured", "smsc credentials are not configured")
		return
	}
	bal, err := h.SMSC.Balance(r.Context(), "admin-balance")
	if err != nil {
		httpx.WriteError(w, http.StatusBadGateway, "smsc_error", err.Error())
		return
	}
	if h.SMSCCache != nil {
		_ = h.SMSCCache.Write(r.Context(), bal)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"provider_code": bal.ProviderCode,
		"balance":       bal.Balance,
		"currency":      bal.Currency,
	})
}

func (h *Handlers) SMSCConnectivityTest(w http.ResponseWriter, r *http.Request) {
	if h.SMSC == nil {
		httpx.WriteError(w, http.StatusConflict, "not_configured", "smsc adapter is not configured")
		return
	}
	probe := h.SMSC.ProbeConnectivity(r.Context())
	status := http.StatusOK
	if !probe.Configured || !probe.BalanceOK || !probe.SignatureOK {
		status = http.StatusConflict
	}
	if probe.BalanceOK && h.SMSCCache != nil {
		_ = h.SMSCCache.Write(r.Context(), smsc.Balance{
			ProviderCode: smsc.ProviderCode,
			Balance:      probe.Balance,
			Currency:     probe.Currency,
		})
	}
	httpx.WriteJSON(w, status, probe)
}

func adminPathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

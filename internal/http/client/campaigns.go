package client

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/campaigns"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
)

const maxRecipientUpload = 8 << 20

type createCampaignRequest struct {
	From string `json:"from"`
	Text string `json:"text"`
}

type patchCampaignRequest struct {
	From *string `json:"from"`
	Text *string `json:"text"`
}

type addRecipientsRequest struct {
	Recipients []string `json:"recipients"`
}

func (h *Handlers) CreateCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	var req createCampaignRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	c, err := h.Campaigns.Create(r.Context(), campaigns.CreateInput{
		ClientID:  *p.ClientID,
		CreatedBy: p.ClientUserID,
		From:      req.From,
		Text:      req.Text,
	})
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	actorType, actorID := p.AuditActor()
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    actorType,
		ActorID:      actorID,
		ClientID:     p.ClientID,
		Action:       "campaign.create",
		ResourceType: "sms_campaign",
		ResourceID:   &c.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"from": c.FromMsisdn},
	})
	httpx.WriteJSON(w, http.StatusCreated, campaignJSON(c, sqlcdb.CampaignRecipientStatsRow{}))
}

func (h *Handlers) ListCampaigns(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	f := campaigns.ListFilter{Limit: 50}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = int32(n)
		}
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		st := sqlcdb.CampaignStatus(raw)
		switch st {
		case sqlcdb.CampaignStatusDraft, sqlcdb.CampaignStatusQueued, sqlcdb.CampaignStatusRunning,
			sqlcdb.CampaignStatusCompleted, sqlcdb.CampaignStatusFailed, sqlcdb.CampaignStatusCancelled:
			f.Status = &st
		default:
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid status")
			return
		}
	}
	items, err := h.Campaigns.List(r.Context(), *p.ClientID, f)
	if err != nil {
		h.Log.Error("list campaigns", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, c := range items {
		out = append(out, campaignJSON(c, sqlcdb.CampaignRecipientStatsRow{}))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) GetCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	c, st, err := h.Campaigns.Get(r.Context(), *p.ClientID, id)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, campaignJSON(c, st))
}

func (h *Handlers) PatchCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	var req patchCampaignRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	c, err := h.Campaigns.Patch(r.Context(), *p.ClientID, id, campaigns.PatchInput{From: req.From, Text: req.Text})
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, campaignJSON(c, sqlcdb.CampaignRecipientStatsRow{}))
}

func (h *Handlers) DeleteCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	if err := h.Campaigns.DeleteDraft(r.Context(), *p.ClientID, id); err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) AddRecipients(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	var req addRecipientsRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if len(req.Recipients) > 10000 {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "at most 10000 recipients per request")
		return
	}
	msisdns, invalid := campaigns.NormalizeRecipientList(req.Recipients)
	rep, err := h.Campaigns.AddRecipients(r.Context(), *p.ClientID, id, msisdns, invalid)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handlers) UploadRecipients(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRecipientUpload)
	if err := r.ParseMultipartForm(maxRecipientUpload); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "file required")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "file field required")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxRecipientUpload+1))
	if err != nil || len(raw) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "cannot read file")
		return
	}
	parsed := campaigns.ParseRecipientFile(raw)
	rep, err := h.Campaigns.AddRecipients(r.Context(), *p.ClientID, id, parsed.MSISDNs, parsed.Invalid)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	rep.Encoding = parsed.Encoding
	httpx.WriteJSON(w, http.StatusOK, rep)
}

func (h *Handlers) ListRecipients(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	limit, offset := int32(50), int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			offset = int32(n)
		}
	}
	items, err := h.Campaigns.ListRecipients(r.Context(), *p.ClientID, id, limit, offset)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, rec := range items {
		row := map[string]any{
			"id":         rec.ID,
			"to":         rec.ToMsisdn,
			"status":     rec.Status,
			"created_at": rec.CreatedAt.UTC().Format(time.RFC3339),
		}
		if rec.SmsMessageID != nil {
			row["message_id"] = *rec.SmsMessageID
		}
		if rec.MessageStatus.Valid {
			row["message_status"] = rec.MessageStatus.SmsStatus
		}
		out = append(out, row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) StartCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	if h.Limiter == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return
	}
	lockKey := "campaign:start:" + p.ClientID.String()
	okLock, err := h.Limiter.TryLock(r.Context(), lockKey, 15*time.Second)
	if err != nil {
		h.Log.Error("campaign start lock", "err", err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return
	}
	if !okLock {
		w.Header().Set("Retry-After", "1")
		httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "campaign start already in progress")
		return
	}
	defer func() { _ = h.Limiter.Unlock(r.Context(), lockKey) }()

	c, err := h.Campaigns.Start(r.Context(), *p.ClientID, id)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	actorType, actorID := p.AuditActor()
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    actorType,
		ActorID:      actorID,
		ClientID:     p.ClientID,
		Action:       "campaign.start",
		ResourceType: "sms_campaign",
		ResourceID:   &c.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"from": c.FromMsisdn, "total": c.TotalCount},
	})
	httpx.WriteJSON(w, http.StatusAccepted, campaignJSON(c, sqlcdb.CampaignRecipientStatsRow{}))
}

func (h *Handlers) CancelCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	c, err := h.Campaigns.Cancel(r.Context(), *p.ClientID, id)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	actorType, actorID := p.AuditActor()
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    actorType,
		ActorID:      actorID,
		ClientID:     p.ClientID,
		Action:       "campaign.cancel",
		ResourceType: "sms_campaign",
		ResourceID:   &c.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"from": c.FromMsisdn, "total": c.TotalCount},
	})
	httpx.WriteJSON(w, http.StatusOK, campaignJSON(c, sqlcdb.CampaignRecipientStatsRow{}))
}

func (h *Handlers) requireCampaigns(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := requireClient(w, r)
	if !ok {
		return identity.Principal{}, false
	}
	if h.Campaigns == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "campaigns unavailable")
		return identity.Principal{}, false
	}
	return p, true
}

func requireClient(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.ClientID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return identity.Principal{}, false
	}
	return p, true
}

func pathCampaignID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

func writeCampaignErr(w http.ResponseWriter, h *Handlers, err error) {
	if httpx.WriteBillingError(w, err) {
		return
	}
	switch {
	case errors.Is(err, campaigns.ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, campaigns.ErrNotAssigned):
		httpx.WriteError(w, http.StatusForbidden, "not_assigned", "from number is not assigned to this client")
	case errors.Is(err, campaigns.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "campaign not found")
	case errors.Is(err, campaigns.ErrFrozen), errors.Is(err, campaigns.ErrNotDraft):
		httpx.WriteError(w, http.StatusConflict, "frozen", "from and text are immutable after start")
	case errors.Is(err, campaigns.ErrEmpty):
		httpx.WriteError(w, http.StatusConflict, "empty", "campaign has no recipients")
	case errors.Is(err, campaigns.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", "campaign cannot be changed in this status")
	case errors.Is(err, campaigns.ErrTooMany):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	default:
		h.Log.Error("campaigns", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

func campaignJSON(c sqlcdb.SmsCampaign, st sqlcdb.CampaignRecipientStatsRow) map[string]any {
	out := map[string]any{
		"id":              c.ID,
		"from":            c.FromMsisdn,
		"text":            c.Text,
		"status":          c.Status,
		"total_count":     c.TotalCount,
		"accepted_count":  c.AcceptedCount,
		"delivered_count": c.DeliveredCount,
		"failed_count":    c.FailedCount,
		"created_at":      c.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":      c.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if st.Total > 0 || st.Pending > 0 || st.Enqueued > 0 {
		out["recipients"] = map[string]any{
			"total":    st.Total,
			"pending":  st.Pending,
			"enqueued": st.Enqueued,
			"skipped":  st.Skipped,
			"failed":   st.Failed,
		}
	}
	return out
}

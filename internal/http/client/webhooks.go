package client

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finenumbers/sms/internal/audit"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/webhooks"
)

func (h *Handlers) requireWebhooks(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := requireClient(w, r)
	if !ok {
		return identity.Principal{}, false
	}
	if h.Webhooks == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "webhooks unavailable")
		return identity.Principal{}, false
	}
	return p, true
}

type webhookCreateRequest struct {
	URL         string   `json:"url"`
	Description *string  `json:"description"`
	Events      []string `json:"events"`
	Enabled     *bool    `json:"enabled"`
}

type webhookPatchRequest struct {
	URL         *string   `json:"url"`
	Description *string   `json:"description"`
	Events      *[]string `json:"events"`
	Enabled     *bool     `json:"enabled"`
}

func (h *Handlers) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	rows, total, err := h.Webhooks.List(r.Context(), *p.ClientID, limit, offset)
	if err != nil {
		h.Log.Error("list webhooks", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, webhooks.EndpointJSON(row))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handlers) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	var req webhookCreateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	row, secret, err := h.Webhooks.Create(r.Context(), webhooks.CreateInput{
		ClientID:    *p.ClientID,
		URL:         req.URL,
		Description: req.Description,
		Events:      req.Events,
		Enabled:     req.Enabled,
	})
	if err != nil {
		webhooks.WriteError(w, err)
		return
	}
	h.auditWebhook(r, p, "webhook.created", &row.ID, map[string]any{"url": row.Url, "events": row.Events})
	httpx.WriteJSON(w, http.StatusCreated, webhooks.EndpointCreatedJSON(row, secret))
}

func (h *Handlers) GetWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "webhookID")
	if !ok {
		return
	}
	row, err := h.Webhooks.GetForClient(r.Context(), *p.ClientID, id)
	if err != nil {
		webhooks.WriteError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, webhooks.EndpointJSON(row))
}

func (h *Handlers) PatchWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "webhookID")
	if !ok {
		return
	}
	var req webhookPatchRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	row, err := h.Webhooks.Patch(r.Context(), *p.ClientID, id, webhooks.PatchInput{
		URL:         req.URL,
		Description: req.Description,
		Events:      req.Events,
		Enabled:     req.Enabled,
	})
	if err != nil {
		webhooks.WriteError(w, err)
		return
	}
	h.auditWebhook(r, p, "webhook.updated", &row.ID, map[string]any{"enabled": row.Enabled, "events": row.Events})
	httpx.WriteJSON(w, http.StatusOK, webhooks.EndpointJSON(row))
}

func (h *Handlers) RotateWebhookSecret(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "webhookID")
	if !ok {
		return
	}
	row, secret, err := h.Webhooks.RotateSecret(r.Context(), *p.ClientID, id)
	if err != nil {
		webhooks.WriteError(w, err)
		return
	}
	h.auditWebhook(r, p, "webhook.secret_rotated", &row.ID, nil)
	httpx.WriteJSON(w, http.StatusOK, webhooks.EndpointCreatedJSON(row, secret))
}

func (h *Handlers) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "webhookID")
	if !ok {
		return
	}
	if err := h.Webhooks.Delete(r.Context(), *p.ClientID, id); err != nil {
		webhooks.WriteError(w, err)
		return
	}
	h.auditWebhook(r, p, "webhook.deleted", &id, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireWebhooks(w, r)
	if !ok {
		return
	}
	limit, offset := lookup.PageFromRequest(r)
	var endpointID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("endpoint_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid endpoint_id")
			return
		}
		endpointID = &id
	}
	if raw := chi.URLParam(r, "webhookID"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
			return
		}
		if _, err := h.Webhooks.GetForClient(r.Context(), *p.ClientID, id); err != nil {
			webhooks.WriteError(w, err)
			return
		}
		endpointID = &id
	}
	var status *sqlcdb.WebhookDeliveryStatus
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		st := sqlcdb.WebhookDeliveryStatus(raw)
		switch st {
		case sqlcdb.WebhookDeliveryStatusPending, sqlcdb.WebhookDeliveryStatusDelivered,
			sqlcdb.WebhookDeliveryStatusFailed, sqlcdb.WebhookDeliveryStatusDead:
			status = &st
		default:
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid status")
			return
		}
	}
	rows, total, err := h.Webhooks.ListDeliveries(r.Context(), *p.ClientID, endpointID, status, limit, offset)
	if err != nil {
		h.Log.Error("list webhook deliveries", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, webhooks.DeliveryJSON(row))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "total": total})
}

func (h *Handlers) auditWebhook(r *http.Request, p identity.Principal, action string, id *uuid.UUID, meta map[string]any) {
	if h.Audit == nil {
		return
	}
	actorType, actorID := p.AuditActor()
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    actorType,
		ActorID:      actorID,
		ClientID:     p.ClientID,
		Action:       action,
		ResourceType: "webhook_endpoint",
		ResourceID:   id,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     meta,
	})
}

package admin

import (
	"errors"
	"net/http"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/audit"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
)

type createAPIKeyRequest struct {
	Name         string   `json:"name"`
	Scopes       []string `json:"scopes"`
	AllowedCIDRs []string `json:"allowed_cidrs"`
}

func (h *Handlers) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if h.Keys == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "api keys unavailable")
		return
	}
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	items, err := h.Keys.List(r.Context(), clientID)
	if err != nil {
		writeAPIKeyErr(w, h, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, k := range items {
		out = append(out, k.JSON())
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Keys == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "api keys unavailable")
		return
	}
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req createAPIKeyRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	created, err := h.Keys.Create(r.Context(), apikeys.CreateInput{
		ClientID:     clientID,
		Name:         req.Name,
		Scopes:       req.Scopes,
		AllowedCIDRs: req.AllowedCIDRs,
		CreatedBy:    *p.AdminUserID,
	})
	if err != nil {
		writeAPIKeyErr(w, h, err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &clientID,
		Action:       "apikey.create",
		ResourceType: "api_credential",
		ResourceID:   &created.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"key_prefix": created.KeyPrefix, "name": created.Name},
	})
	body := created.JSON()
	body["token"] = created.Token
	httpx.WriteJSON(w, http.StatusCreated, body)
}

func (h *Handlers) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Keys == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "api keys unavailable")
		return
	}
	clientID, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	keyID, ok := pathUUID(w, r, "keyID")
	if !ok {
		return
	}
	out, err := h.Keys.Revoke(r.Context(), clientID, keyID)
	if err != nil {
		writeAPIKeyErr(w, h, err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &clientID,
		Action:       "apikey.revoke",
		ResourceType: "api_credential",
		ResourceID:   &out.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"key_prefix": out.KeyPrefix},
	})
	httpx.WriteJSON(w, http.StatusOK, out.JSON())
}

func writeAPIKeyErr(w http.ResponseWriter, h *Handlers, err error) {
	switch {
	case errors.Is(err, apikeys.ErrValidation), errors.Is(err, identity.ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, apikeys.ErrNotFound), errors.Is(err, identity.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, apikeys.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", err.Error())
	default:
		h.Log.Error("api keys", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

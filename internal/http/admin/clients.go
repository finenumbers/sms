package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/inventory"
)

type createClientRequest struct {
	Name          string `json:"name"`
	OwnerEmail    string `json:"owner_email"`
	OwnerPassword string `json:"owner_password"`
}

type patchClientRequest struct {
	Name *string `json:"name"`
}

type resetOwnerPasswordRequest struct {
	Password string `json:"password"`
}

func (h *Handlers) CreateClient(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.AdminUserID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req createClientRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	out, err := h.Ident.CreateClient(r.Context(), identity.CreateClientInput{
		Name:          req.Name,
		OwnerEmail:    req.OwnerEmail,
		OwnerPassword: req.OwnerPassword,
		CreatedBy:     *p.AdminUserID,
	})
	if err != nil {
		writeClientErr(w, h, "create client", err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &out.Client.ID,
		Action:       "client.create",
		ResourceType: "client",
		ResourceID:   &out.Client.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"owner_email": out.Owner.Email},
	})
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"id":          out.Client.ID,
		"name":        out.Client.Name,
		"status":      out.Client.Status,
		"owner_id":    out.Owner.ID,
		"owner_email": out.Owner.Email,
		"owner_role":  out.Owner.Role,
		"created_at":  out.Client.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *Handlers) ListClients(w http.ResponseWriter, r *http.Request) {
	limit, offset := queryPage(r)
	var status *sqlcdb.ClientStatus
	if raw := r.URL.Query().Get("status"); raw != "" {
		st := sqlcdb.ClientStatus(raw)
		switch st {
		case sqlcdb.ClientStatusActive, sqlcdb.ClientStatusSuspended:
			status = &st
		default:
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid status")
			return
		}
	}
	items, err := h.Ident.ListClients(r.Context(), status, limit, offset)
	if err != nil {
		h.Log.Error("list clients", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, c := range items {
		row := map[string]any{
			"id":             c.ID,
			"name":           c.Name,
			"status":         c.Status,
			"owner_email":    c.OwnerEmail,
			"assigned_count": c.AssignedCount,
			"created_at":     c.CreatedAt.UTC().Format(time.RFC3339),
		}
		if c.AvailableBalance != nil {
			row["available_balance"] = billing.FormatMoney(*c.AvailableBalance)
		}
		if c.HeldBalance != nil {
			row["held_balance"] = billing.FormatMoney(*c.HeldBalance)
		}
		if c.WalletCurrency != nil {
			row["currency"] = *c.WalletCurrency
		}
		out = append(out, row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) GetClient(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	d, err := h.Ident.GetClient(r.Context(), id)
	if err != nil {
		writeClientErr(w, h, "get client", err)
		return
	}
	users := make([]map[string]any, 0, len(d.Users))
	for _, u := range d.Users {
		users = append(users, map[string]any{
			"id":         u.ID,
			"email":      u.Email,
			"role":       u.Role,
			"status":     u.Status,
			"created_at": u.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	body := map[string]any{
		"id":         d.Client.ID,
		"name":       d.Client.Name,
		"status":     d.Client.Status,
		"created_at": d.Client.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": d.Client.UpdatedAt.UTC().Format(time.RFC3339),
		"users":      users,
	}
	if h.Store != nil {
		if wlt, err := h.Store.Queries.GetWalletByClientID(r.Context(), id); err == nil {
			body["available_balance"] = billing.FormatMoney(wlt.AvailableBalance)
			body["held_balance"] = billing.FormatMoney(wlt.HeldBalance)
			body["currency"] = wlt.Currency
		}
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handlers) PatchClient(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req patchClientRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if req.Name == nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "name required")
		return
	}
	cl, err := h.Ident.UpdateClient(r.Context(), id, *req.Name)
	if err != nil {
		writeClientErr(w, h, "update client", err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &cl.ID,
		Action:       "client.update",
		ResourceType: "client",
		ResourceID:   &cl.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"name": cl.Name},
	})
	httpx.WriteJSON(w, http.StatusOK, clientStatusJSON(cl))
}

func (h *Handlers) SuspendClient(w http.ResponseWriter, r *http.Request) {
	h.changeClientStatus(w, r, "client.suspend", func(id uuid.UUID) (sqlcdb.Client, error) {
		return h.Ident.SuspendClient(r.Context(), id)
	})
}

func (h *Handlers) ActivateClient(w http.ResponseWriter, r *http.Request) {
	h.changeClientStatus(w, r, "client.activate", func(id uuid.UUID) (sqlcdb.Client, error) {
		return h.Ident.ActivateClient(r.Context(), id)
	})
}

func (h *Handlers) DeleteClient(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var afterLock func(context.Context, *sqlcdb.Queries) error
	if h.Inventory != nil {
		afterLock = func(ctx context.Context, q *sqlcdb.Queries) error {
			_, err := inventory.UnassignAll(ctx, q, id)
			return err
		}
	}
	cl, err := h.Ident.DeleteClientAnd(r.Context(), id, afterLock)
	if err != nil {
		writeClientErr(w, h, "delete client", err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &cl.ID,
		Action:       "client.delete",
		ResourceType: "client",
		ResourceID:   &cl.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) ResetOwnerPassword(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req resetOwnerPasswordRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if err := h.Ident.ResetOwnerPassword(r.Context(), id, req.Password); err != nil {
		writeClientErr(w, h, "reset owner password", err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &id,
		Action:       "client.password.reset",
		ResourceType: "client",
		ResourceID:   &id,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) changeClientStatus(w http.ResponseWriter, r *http.Request, action string, fn func(uuid.UUID) (sqlcdb.Client, error)) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	cl, err := fn(id)
	if err != nil {
		writeClientErr(w, h, action, err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &cl.ID,
		Action:       action,
		ResourceType: "client",
		ResourceID:   &cl.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"status": cl.Status},
	})
	httpx.WriteJSON(w, http.StatusOK, clientStatusJSON(cl))
}

func writeClientErr(w http.ResponseWriter, h *Handlers, op string, err error) {
	switch {
	case errors.Is(err, identity.ErrEmailTaken):
		httpx.WriteError(w, http.StatusConflict, "email_taken", "owner email already in use")
	case errors.Is(err, identity.ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, identity.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "client not found")
	case errors.Is(err, identity.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", err.Error())
	default:
		h.Log.Error(op, "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

func clientStatusJSON(c sqlcdb.Client) map[string]any {
	return map[string]any{
		"id":         c.ID,
		"name":       c.Name,
		"status":     c.Status,
		"created_at": c.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func requireAdmin(w http.ResponseWriter, r *http.Request) (identity.Principal, bool) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.AdminUserID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return identity.Principal{}, false
	}
	return p, true
}

func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return uuid.UUID{}, false
	}
	return id, true
}

func queryPage(r *http.Request) (int32, int32) {
	limit := int32(50)
	offset := int32(0)
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
	return limit, offset
}

package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
)

func (h *Handlers) ListCallbacks(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "store unavailable")
		return
	}
	arg := sqlcdb.ListCallbackEventsParams{PageLimit: 50, PageOffset: 0}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			arg.PageLimit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			arg.PageOffset = int32(n)
		}
	}
	if raw := r.URL.Query().Get("kind"); raw != "" {
		k := sqlcdb.CallbackKind(raw)
		if k != sqlcdb.CallbackKindDlr && k != sqlcdb.CallbackKindMo {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid kind")
			return
		}
		arg.Kind = sqlcdb.NullCallbackKind{CallbackKind: k, Valid: true}
	}
	if arg.PageLimit <= 0 || arg.PageLimit > 100 {
		arg.PageLimit = 50
	}
	items, err := h.Store.Queries.ListCallbackEvents(r.Context(), arg)
	if err != nil {
		h.Log.Error("list callbacks", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		out = append(out, map[string]any{
			"id":              e.ID,
			"kind":            e.Kind,
			"method":          e.Method,
			"path":            e.Path,
			"content_type":    e.ContentType,
			"body_bytes":      e.BodyBytes,
			"created_at":      e.CreatedAt.UTC().Format(time.RFC3339),
			"processed_at":    e.ProcessedAt,
			"idempotency_key": e.IdempotencyKey,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) GetCallback(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "store unavailable")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "callbackID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	e, err := h.Store.Queries.GetCallbackEventByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	raw := e.RawBody
	var rawOut any
	if utf8.Valid(raw) {
		rawOut = string(raw)
	} else {
		rawOut = raw
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":              e.ID,
		"kind":            e.Kind,
		"method":          e.Method,
		"path":            e.Path,
		"query":           e.Query,
		"content_type":    e.ContentType,
		"headers":         jsonRaw(e.Headers),
		"raw_body":        rawOut,
		"created_at":      e.CreatedAt.UTC().Format(time.RFC3339),
		"processed_at":    e.ProcessedAt,
		"idempotency_key": e.IdempotencyKey,
	})
}

func jsonRaw(b []byte) any {
	if len(b) == 0 {
		return map[string]any{}
	}
	return json.RawMessage(b)
}

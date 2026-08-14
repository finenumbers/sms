package ingresshttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/ratelimit"
)

type TokenLookup interface {
	IngressHash(ctx context.Context) (string, error)
}

type EventWriter interface {
	InsertCallbackEvent(ctx context.Context, arg sqlcdb.InsertCallbackEventParams) (sqlcdb.ProviderCallbackEvent, error)
}

type Handlers struct {
	Log     *slog.Logger
	Events  EventWriter
	Tokens  TokenLookup
	Limiter *ratelimit.Limiter
	Ops     *ops.Logger
}

func (h *Handlers) Capture(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost, http.MethodPut:
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	kind, ok := ingress.KindFromPath(chi.URLParam(r, "kind"))
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	token := chi.URLParam(r, "token")
	if h.Tokens == nil || h.Events == nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	stored, err := h.Tokens.IngressHash(r.Context())
	if err != nil || stored == "" || !ingress.TokenMatch(stored, token) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if h.Limiter != nil {
		ip := "unknown"
		if v := httpx.ClientIP(r); v != nil {
			ip = v.String()
		}
		ok, retry, err := h.Limiter.Allow(r.Context(), "rl:ingress:"+ip, 200, time.Second)
		if err == nil && !ok {
			if retry > 0 {
				w.Header().Set("Retry-After", "1")
			}
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
	}

	body, err := ingress.ReadBody(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "cannot read body")
		return
	}
	if len(body) > ingress.MaxBody {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "body too large")
		return
	}

	query := r.URL.RawQuery
	var qp *string
	if query != "" {
		qp = &query
	}
	ct := strings.TrimSpace(r.Header.Get("Content-Type"))
	var ctp *string
	if ct != "" {
		ctp = &ct
	}
	ev, err := h.Events.InsertCallbackEvent(r.Context(), sqlcdb.InsertCallbackEventParams{
		Kind:           kind,
		IdempotencyKey: ingress.IdempotencyKey(r.Method, r.URL.Path, query, body),
		Method:         r.Method,
		Path:           ingress.RedactPath(r.URL.Path),
		Query:          qp,
		Headers:        ingress.SanitizeHeaders(r.Header),
		RawBody:        body,
		ContentType:    ctp,
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		if h.Log != nil {
			h.Log.Error("ingress persist", "err", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err == nil && h.Ops != nil {
		h.Ops.Write(r.Context(), ops.Event{
			Category:     ops.CategoryIngress,
			Level:        ops.LevelInfo,
			Action:       "ingress.received",
			ResourceType: "provider_callback_event",
			ResourceID:   &ev.ID,
			HTTPMethod:   r.Method,
			HTTPPath:     ingress.RedactPath(r.URL.Path),
			HTTPStatus:   http.StatusOK,
			Summary:      string(kind),
			Detail: map[string]any{
				"id":         ev.ID,
				"kind":       kind,
				"method":     r.Method,
				"path":       ingress.RedactPath(r.URL.Path),
				"body_bytes": len(body),
			},
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
}

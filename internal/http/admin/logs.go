package admin

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/ops"
)

const (
	logsDefaultLimit  = 50
	logsMaxLimit      = 100
	logsMaxOffset     = 1000
	logsDefaultWindow = time.Hour
	logsMaxSpan       = 24 * time.Hour
)

func (h *Handlers) ListLogs(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "store unavailable")
		return
	}
	now := time.Now().UTC()
	from, to, err := parseLogsWindow(r.URL.Query().Get("from"), r.URL.Query().Get("to"), now)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	arg := sqlcdb.ListOpsEventsParams{
		FromTs:     from,
		ToTs:       to,
		PageLimit:  logsDefaultLimit,
		PageOffset: 0,
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n <= 0 || n > logsMaxLimit {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "limit must be 1..100")
			return
		}
		arg.PageLimit = int32(n)
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 0 || n > logsMaxOffset {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "offset must be 0..1000")
			return
		}
		arg.PageOffset = int32(n)
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("category")); raw != "" {
		if !validOpsCategory(raw) {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid category")
			return
		}
		arg.Category = &raw
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("level")); raw != "" {
		if raw != ops.LevelInfo && raw != ops.LevelWarn && raw != ops.LevelError {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid level")
			return
		}
		arg.Level = &raw
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("request_id")); raw != "" {
		arg.RequestID = &raw
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("client_id")); raw != "" {
		id, parseErr := uuid.Parse(raw)
		if parseErr != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid client_id")
			return
		}
		arg.ClientID = &id
	}
	if q := likeContains(r.URL.Query().Get("q")); q != "" {
		arg.Q = &q
	}
	items, err := h.Store.Queries.ListOpsEvents(r.Context(), arg)
	if err != nil {
		h.Log.Error("list logs", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		out = append(out, opsListItem(e))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items": out,
		"from":  from.Format(time.RFC3339),
		"to":    to.Format(time.RFC3339),
	})
}

func (h *Handlers) GetLog(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "store unavailable")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "logID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	e, err := h.Store.Queries.GetOpsEventByID(r.Context(), id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, opsDetailItem(e))
}

func parseLogsWindow(fromRaw, toRaw string, now time.Time) (time.Time, time.Time, error) {
	to := now
	from := now.Add(-logsDefaultWindow)
	if toRaw != "" {
		t, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			return time.Time{}, time.Time{}, errString("invalid to")
		}
		to = t.UTC()
	}
	if fromRaw != "" {
		t, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			return time.Time{}, time.Time{}, errString("invalid from")
		}
		from = t.UTC()
	} else if toRaw != "" {
		from = to.Add(-logsDefaultWindow)
	}
	if !from.Before(to) {
		return time.Time{}, time.Time{}, errString("from must be before to")
	}
	if to.Sub(from) > logsMaxSpan {
		return time.Time{}, time.Time{}, errString("time span must be at most 24h")
	}
	return from, to, nil
}

type errString string

func (e errString) Error() string { return string(e) }

func validOpsCategory(s string) bool {
	switch s {
	case ops.CategoryHTTP, ops.CategoryDIDAPI, ops.CategoryQueue, ops.CategoryIngress, ops.CategoryAudit:
		return true
	default:
		return false
	}
}

func likeContains(q string) string {
	q = strings.ReplaceAll(q, `\`, "")
	q = strings.ReplaceAll(q, `%`, "")
	q = strings.ReplaceAll(q, `_`, "")
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	return "%" + q + "%"
}

func opsListItem(e sqlcdb.ListOpsEventsRow) map[string]any {
	m := map[string]any{
		"id":         e.ID,
		"created_at": e.CreatedAt.UTC().Format(time.RFC3339),
		"category":   e.Category,
		"level":      e.Level,
		"action":     e.Action,
	}
	if e.RequestID != nil {
		m["request_id"] = *e.RequestID
	}
	if e.ActorType.Valid {
		m["actor_type"] = e.ActorType.ActorType
	}
	if e.ActorID != nil {
		m["actor_id"] = e.ActorID
	}
	if e.ClientID != nil {
		m["client_id"] = e.ClientID
	}
	if e.ResourceType != nil {
		m["resource_type"] = *e.ResourceType
	}
	if e.ResourceID != nil {
		m["resource_id"] = e.ResourceID
	}
	if e.HttpMethod != nil {
		m["http_method"] = *e.HttpMethod
	}
	if e.HttpPath != nil {
		m["http_path"] = *e.HttpPath
	}
	if e.HttpStatus != nil {
		m["http_status"] = *e.HttpStatus
	}
	if e.LatencyMs != nil {
		m["latency_ms"] = *e.LatencyMs
	}
	if e.Summary != nil {
		m["summary"] = *e.Summary
	}
	if e.Error != nil {
		m["error"] = *e.Error
	}
	if e.Ip != nil {
		m["ip"] = e.Ip.String()
	}
	return m
}

func opsDetailItem(e sqlcdb.OpsEvent) map[string]any {
	m := opsListItem(sqlcdb.ListOpsEventsRow{
		ID:           e.ID,
		CreatedAt:    e.CreatedAt,
		Category:     e.Category,
		Level:        e.Level,
		RequestID:    e.RequestID,
		ActorType:    e.ActorType,
		ActorID:      e.ActorID,
		ClientID:     e.ClientID,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		HttpMethod:   e.HttpMethod,
		HttpPath:     e.HttpPath,
		HttpStatus:   e.HttpStatus,
		LatencyMs:    e.LatencyMs,
		Summary:      e.Summary,
		Error:        e.Error,
		Ip:           e.Ip,
	})
	if len(e.Detail) == 0 {
		m["detail"] = map[string]any{}
		return m
	}
	m["detail"] = json.RawMessage(e.Detail)
	return m
}

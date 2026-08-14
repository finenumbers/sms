package client

import (
	"errors"
	"net/http"
	"time"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/httpx"
)

func (h *Handlers) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	if h.Keys == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "api keys unavailable")
		return
	}
	items, err := h.Keys.List(r.Context(), *p.ClientID)
	if err != nil {
		if errors.Is(err, apikeys.ErrNotFound) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		h.Log.Error("list api keys", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, k := range items {
		row := map[string]any{
			"id":         k.ID,
			"name":       k.Name,
			"key_prefix": k.KeyPrefix,
			"status":     k.Status,
			"created_at": k.CreatedAt.UTC().Format(time.RFC3339),
		}
		if k.LastUsedAt != nil {
			row["last_used_at"] = k.LastUsedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

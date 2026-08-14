package client

import (
	"net/http"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/inventory"
)

func (h *Handlers) ListNumbers(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	if h.Inventory == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "inventory unavailable")
		return
	}
	st := sqlcdb.DefNumberStatusAssigned
	items, err := h.Inventory.List(r.Context(), inventory.ListFilter{
		ClientID: p.ClientID,
		Status:   &st,
		Limit:    100,
	})
	if err != nil {
		h.Log.Error("list client numbers", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, n := range items {
		out = append(out, map[string]any{
			"id":     n.ID,
			"msisdn": n.Msisdn,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

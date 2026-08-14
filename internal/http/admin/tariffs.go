package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
)

func (h *Handlers) ListTariffs(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	limit, offset := queryPage(r)
	rows, err := h.Store.Queries.ListTariffPlans(r.Context(), sqlcdb.ListTariffPlansParams{
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		h.Log.Error("list tariffs", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, tariffJSON(row))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

type tariffRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Product     string `json:"product"`
	SellPrice   string `json:"sell_price"`
	Currency    string `json:"currency"`
	IsDefault   bool   `json:"is_default"`
	IsActive    *bool  `json:"is_active"`
	Description string `json:"description"`
}

func (h *Handlers) CreateTariff(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	var req tariffRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	product := sqlcdb.BillingProduct(req.Product)
	if !billing.KnownProduct(product) {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid product")
		return
	}
	price, err := decimal.NewFromString(req.SellPrice)
	if err != nil || !price.IsPositive() {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid sell_price")
		return
	}
	cur := req.Currency
	if cur == "" {
		cur = "RUB"
	}
	active := true
	if req.IsActive != nil {
		active = *req.IsActive
	}
	var desc *string
	if req.Description != "" {
		desc = &req.Description
	}
	row, err := h.Store.Queries.InsertTariffPlan(r.Context(), sqlcdb.InsertTariffPlanParams{
		Code:        req.Code,
		Name:        req.Name,
		Product:     product,
		SellPrice:   price,
		Currency:    cur,
		IsDefault:   req.IsDefault,
		IsActive:    active,
		Description: desc,
	})
	if err != nil {
		h.Log.Error("create tariff", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "billing.tariff.create",
		ResourceType: "tariff_plan",
		ResourceID:   &row.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"code": row.Code, "product": row.Product},
	})
	httpx.WriteJSON(w, http.StatusCreated, tariffJSON(row))
}

func (h *Handlers) PatchTariff(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.AdminUserID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id, ok := pathUUID(w, r, "tariffID")
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	cur, err := h.Store.Queries.GetTariffPlanByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "tariff not found")
			return
		}
		h.Log.Error("get tariff", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	var req tariffRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	name := cur.Name
	if req.Name != "" {
		name = req.Name
	}
	price := cur.SellPrice
	if req.SellPrice != "" {
		d, err := decimal.NewFromString(req.SellPrice)
		if err != nil || !d.IsPositive() {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid sell_price")
			return
		}
		price = d
	}
	active := cur.IsActive
	if req.IsActive != nil {
		active = *req.IsActive
	}
	desc := cur.Description
	if req.Description != "" {
		desc = &req.Description
	}
	row, err := h.Store.Queries.UpdateTariffPlan(r.Context(), sqlcdb.UpdateTariffPlanParams{
		Name:        name,
		SellPrice:   price,
		IsDefault:   req.IsDefault,
		IsActive:    active,
		Description: desc,
		ID:          id,
	})
	if err != nil {
		h.Log.Error("patch tariff", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "billing.tariff.update",
		ResourceType: "tariff_plan",
		ResourceID:   &row.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
	})
	httpx.WriteJSON(w, http.StatusOK, tariffJSON(row))
}

func tariffJSON(row sqlcdb.TariffPlan) map[string]any {
	out := map[string]any{
		"id":         row.ID,
		"code":       row.Code,
		"name":       row.Name,
		"product":    row.Product,
		"sell_price": billing.FormatMoney(row.SellPrice),
		"currency":   row.Currency,
		"is_default": row.IsDefault,
		"is_active":  row.IsActive,
		"created_at": row.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at": row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.Description != nil {
		out["description"] = *row.Description
	}
	return out
}

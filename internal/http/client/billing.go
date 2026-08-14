package client

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/msisdn"
)

func (h *Handlers) GetBalance(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	body, err := h.balanceJSON(r.Context(), *p.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "wallet not found")
			return
		}
		h.Log.Error("balance", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handlers) balanceJSON(ctx context.Context, clientID uuid.UUID) (map[string]any, error) {
	if h.Store == nil {
		return nil, errors.New("store unavailable")
	}
	wlt, err := h.Store.Queries.GetWalletByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"available_balance": billing.FormatMoney(wlt.AvailableBalance),
		"held_balance":      billing.FormatMoney(wlt.HeldBalance),
		"currency":          wlt.Currency,
	}, nil
}

func (h *Handlers) GetLedger(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	limit, offset := int32(50), int32(0)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseInt32(v); err == nil {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := parseInt32(v); err == nil {
			offset = n
		}
	}
	rows, err := h.Store.Queries.ListWalletTransactionsForClient(r.Context(), sqlcdb.ListWalletTransactionsForClientParams{
		ClientID:   *p.ClientID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		h.Log.Error("client ledger", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":         row.ID,
			"type":       row.Type,
			"amount":     billing.FormatMoney(row.Amount),
			"currency":   row.Currency,
			"created_at": row.CreatedAt.UTC().Format(time.RFC3339),
		}
		if row.BalanceAfterAvailable != nil {
			item["balance_after_available"] = billing.FormatMoney(*row.BalanceAfterAvailable)
		}
		if row.BalanceAfterHeld != nil {
			item["balance_after_held"] = billing.FormatMoney(*row.BalanceAfterHeld)
		}
		if row.SmsMessageID != nil {
			item["sms_message_id"] = *row.SmsMessageID
		}
		if row.Description != nil {
			item["description"] = *row.Description
		}
		items = append(items, item)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) GetTariff(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	rows, err := h.Store.Queries.ListClientTariffs(r.Context(), *p.ClientID)
	if err != nil {
		h.Log.Error("client tariffs", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, t := range rows {
		sell := t.PlanSellPrice
		if t.PriceOverride != nil {
			sell = *t.PriceOverride
		}
		items = append(items, map[string]any{
			"product":    t.Product,
			"plan_name":  t.PlanName,
			"sell_price": billing.FormatMoney(sell),
			"currency":   t.Currency,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	ctx := r.Context()
	now := time.Now().UTC()
	windows := map[string]time.Time{
		"24h": now.Add(-24 * time.Hour),
		"7d":  now.Add(-7 * 24 * time.Hour),
		"30d": now.Add(-30 * 24 * time.Hour),
	}
	out := map[string]any{}
	for label, since := range windows {
		spent, err := h.Store.Queries.SumDebitsSinceForClient(ctx, sqlcdb.SumDebitsSinceForClientParams{
			ClientID: *p.ClientID,
			Since:    since,
		})
		if err != nil {
			h.Log.Error("client stats spend", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		rows, err := h.Store.Queries.CountOutboundSmsSinceByStatusForClient(ctx, sqlcdb.CountOutboundSmsSinceByStatusForClientParams{
			ClientID: p.ClientID,
			Since:    since,
		})
		if err != nil {
			h.Log.Error("client stats sms", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		sms := map[string]int64{}
		for _, row := range rows {
			sms[string(row.Status)] = row.N
		}
		lookups := map[string]int64{}
		if lookupRows, err := h.Store.Queries.CountLookupItemsSinceByTypeForClient(ctx, sqlcdb.CountLookupItemsSinceByTypeForClientParams{
			ClientID: *p.ClientID,
			Since:    since,
		}); err != nil {
			h.Log.Error("client stats lookups", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		} else {
			for _, row := range lookupRows {
				lookups[string(row.CheckType)] = row.N
			}
		}
		out[label] = map[string]any{"spent": billing.FormatMoney(spent), "sms": sms, "lookups": lookups}
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

type estimateRequest struct {
	To   string `json:"to"`
	Text string `json:"text"`
}

func (h *Handlers) Estimate(w http.ResponseWriter, r *http.Request) {
	p, ok := requireClient(w, r)
	if !ok {
		return
	}
	if h.Billing == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	var req estimateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	dest, err := msisdn.NormalizeDest(req.To)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	est, err := h.Billing.Estimate(r.Context(), *p.ClientID, dest, req.Text)
	if err != nil {
		if httpx.WriteBillingError(w, err) {
			return
		}
		h.Log.Error("estimate", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"billed":          true,
		"product":         est.Product,
		"segments":        est.Segments,
		"unit_sell_price": billing.FormatMoney(est.UnitSellPrice),
		"total":           billing.FormatMoney(est.Total),
		"currency":        est.Currency,
	})
}

func (h *Handlers) EstimateCampaign(w http.ResponseWriter, r *http.Request) {
	p, ok := h.requireCampaigns(w, r)
	if !ok {
		return
	}
	id, ok := pathCampaignID(w, r)
	if !ok {
		return
	}
	c, _, err := h.Campaigns.Get(r.Context(), *p.ClientID, id)
	if err != nil {
		writeCampaignErr(w, h, err)
		return
	}
	cost, err := h.Campaigns.CampaignCost(r.Context(), *p.ClientID, c)
	if err != nil {
		if httpx.WriteBillingError(w, err) {
			return
		}
		writeCampaignErr(w, h, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"billed":        true,
		"domestic":      cost.Domestic,
		"international": cost.International,
		"segments":      cost.Segments,
		"total":         billing.FormatMoney(cost.Total),
		"currency":      cost.Currency,
	})
}

func parseInt32(s string) (int32, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not a number")
		}
		n = n*10 + int(c-'0')
	}
	return int32(n), nil
}

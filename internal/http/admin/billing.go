package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
)

func (h *Handlers) BillingOverview(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil || h.Settings == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	ctx := r.Context()
	view, err := h.Settings.Get(ctx)
	if err != nil {
		h.Log.Error("billing overview settings", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	sums, err := h.Store.Queries.SumWalletBalances(ctx)
	if err != nil {
		h.Log.Error("billing overview wallets", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	threshold, err := decimal.NewFromString(view.LowBalanceThreshold)
	if err != nil {
		threshold = decimal.NewFromInt(100)
	}
	low, err := h.Store.Queries.CountLowBalanceClients(ctx, threshold)
	if err != nil {
		h.Log.Error("billing overview low", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	openHolds, err := h.Store.Queries.CountOpenHolds(ctx)
	if err != nil {
		h.Log.Error("billing overview holds", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	now := time.Now().UTC()
	d24 := now.Add(-24 * time.Hour)
	d7 := now.Add(-7 * 24 * time.Hour)
	spent24, err := h.Store.Queries.SumDebitsSince(ctx, d24)
	if err != nil {
		h.Log.Error("billing overview spend24", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	spent7, err := h.Store.Queries.SumDebitsSince(ctx, d7)
	if err != nil {
		h.Log.Error("billing overview spend7", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	pdu24, err := h.Store.Queries.SumBilledSegmentsSince(ctx, d24)
	if err != nil {
		h.Log.Error("billing overview pdu", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	sms24, err := outboundStatusCounts(ctx, h, d24)
	if err != nil {
		h.Log.Error("billing overview sms24", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	sms7, err := outboundStatusCounts(ctx, h, d7)
	if err != nil {
		h.Log.Error("billing overview sms7", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	prod24, err := outboundProductCounts(ctx, h, d24)
	if err != nil {
		h.Log.Error("billing overview prod", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"billing_enforced":      view.BillingEnforced,
		"low_balance_threshold": view.LowBalanceThreshold,
		"available_total":       billing.FormatMoney(sums.AvailableTotal),
		"held_total":            billing.FormatMoney(sums.HeldTotal),
		"spent_24h":             billing.FormatMoney(spent24),
		"spent_7d":              billing.FormatMoney(spent7),
		"low_balance_clients":   low,
		"open_holds":            openHolds,
		"billed_segments_24h":   pdu24,
		"sms_24h":               sms24,
		"sms_7d":                sms7,
		"sms_by_product_24h":    prod24,
	})
}

func outboundStatusCounts(ctx context.Context, h *Handlers, since time.Time) (map[string]int64, error) {
	rows, err := h.Store.Queries.CountOutboundSmsSinceByStatus(ctx, since)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[string(row.Status)] = row.N
	}
	return out, nil
}

func outboundProductCounts(ctx context.Context, h *Handlers, since time.Time) (map[string]int64, error) {
	rows, err := h.Store.Queries.CountOutboundSmsSinceByProduct(ctx, since)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, row := range rows {
		out[row.Product] = row.N
	}
	return out, nil
}

func (h *Handlers) PlatformLedger(w http.ResponseWriter, r *http.Request) {
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	limit, offset := queryPage(r)
	arg := sqlcdb.ListPlatformLedgerParams{PageLimit: limit, PageOffset: offset}
	if raw := r.URL.Query().Get("client_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid client_id")
			return
		}
		arg.ClientID = &id
	}
	if raw := r.URL.Query().Get("type"); raw != "" {
		t := sqlcdb.WalletTxType(raw)
		switch t {
		case sqlcdb.WalletTxTypeCREDIT, sqlcdb.WalletTxTypeHOLD, sqlcdb.WalletTxTypeDEBIT, sqlcdb.WalletTxTypeRELEASE, sqlcdb.WalletTxTypeADJUSTMENT:
			arg.TxType = sqlcdb.NullWalletTxType{WalletTxType: t, Valid: true}
		default:
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid type")
			return
		}
	}
	rows, err := h.Store.Queries.ListPlatformLedger(r.Context(), arg)
	if err != nil {
		h.Log.Error("platform ledger", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, ledgerRowJSON(row.ID, row.ClientID, row.ClientName, row.Type, row.Amount, row.Currency, row.BalanceAfterAvailable, row.BalanceAfterHeld, row.SmsMessageID, row.Description, row.CreatedAt))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handlers) GetClientBilling(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	body, err := h.clientBillingJSON(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "wallet not found")
			return
		}
		h.Log.Error("client billing", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handlers) clientBillingJSON(ctx context.Context, clientID uuid.UUID) (map[string]any, error) {
	if h.Store == nil {
		return nil, errors.New("store unavailable")
	}
	wlt, err := h.Store.Queries.GetWalletByClientID(ctx, clientID)
	if err != nil {
		return nil, err
	}
	tariffs, err := h.Store.Queries.ListClientTariffs(ctx, clientID)
	if err != nil {
		return nil, err
	}
	limit, offset := int32(50), int32(0)
	txs, err := h.Store.Queries.ListWalletTransactionsForClient(ctx, sqlcdb.ListWalletTransactionsForClientParams{
		ClientID:   clientID,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(txs))
	for _, row := range txs {
		items = append(items, ledgerRowJSON(row.ID, row.ClientID, "", row.Type, row.Amount, row.Currency, row.BalanceAfterAvailable, row.BalanceAfterHeld, row.SmsMessageID, row.Description, row.CreatedAt))
	}
	tOut := make([]map[string]any, 0, len(tariffs))
	for _, t := range tariffs {
		tOut = append(tOut, clientTariffJSON(t))
	}
	return map[string]any{
		"available_balance": billing.FormatMoney(wlt.AvailableBalance),
		"held_balance":      billing.FormatMoney(wlt.HeldBalance),
		"currency":          wlt.Currency,
		"tariffs":           tOut,
		"ledger":            items,
	}, nil
}

type moneyRequest struct {
	Amount         string `json:"amount"`
	Comment        string `json:"comment"`
	IdempotencyKey string `json:"idempotency_key"`
	Direction      string `json:"direction"`
	AllowNegative  bool   `json:"allow_negative"`
}

func (h *Handlers) TopUpClient(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Billing == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req moneyRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	err := h.Billing.TopUp(r.Context(), billing.TopUpInput{
		ClientID:       id,
		Amount:         req.Amount,
		Comment:        req.Comment,
		IdempotencyKey: req.IdempotencyKey,
		CreatedBy:      p.AdminUserID,
	})
	if err != nil {
		if httpx.WriteBillingError(w, err) {
			return
		}
		h.Log.Error("topup", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &id,
		Action:       "billing.topup",
		ResourceType: "wallet",
		ResourceID:   &id,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"amount": req.Amount, "comment": req.Comment},
	})
	body, err := h.clientBillingJSON(r.Context(), id)
	if err != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (h *Handlers) AdjustClient(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Billing == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req moneyRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	err := h.Billing.Adjust(r.Context(), billing.AdjustInput{
		ClientID:       id,
		Amount:         req.Amount,
		Direction:      req.Direction,
		Comment:        req.Comment,
		IdempotencyKey: req.IdempotencyKey,
		AllowNegative:  req.AllowNegative,
		CreatedBy:      p.AdminUserID,
	})
	if err != nil {
		if httpx.WriteBillingError(w, err) {
			return
		}
		h.Log.Error("adjust", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &id,
		Action:       "billing.adjust",
		ResourceType: "wallet",
		ResourceID:   &id,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"amount": req.Amount, "direction": req.Direction, "comment": req.Comment},
	})
	body, err := h.clientBillingJSON(r.Context(), id)
	if err != nil {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

type assignTariffRequest struct {
	Product       string `json:"product"`
	TariffPlanID  string `json:"tariff_plan_id"`
	PriceOverride string `json:"price_override"`
}

func (h *Handlers) AssignClientTariff(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	var req assignTariffRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	product := sqlcdb.BillingProduct(req.Product)
	if !billing.KnownProduct(product) {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid product")
		return
	}
	if !clientAssignable(w, r, h, id) {
		return
	}
	planID, err := uuid.Parse(req.TariffPlanID)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid tariff_plan_id")
		return
	}
	plan, err := h.Store.Queries.GetTariffPlanByID(r.Context(), planID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "tariff plan not found")
			return
		}
		h.Log.Error("assign tariff plan", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err := billing.AssertPlanAssignable(plan, product); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return
	}
	arg := sqlcdb.UpsertClientTariffParams{ClientID: id, Product: product, TariffPlanID: planID}
	if req.PriceOverride != "" {
		d, err := decimal.NewFromString(req.PriceOverride)
		if err != nil || !d.IsPositive() {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid price_override")
			return
		}
		arg.PriceOverride = &d
	}
	row, err := h.Store.Queries.UpsertClientTariff(r.Context(), arg)
	if err != nil {
		h.Log.Error("assign tariff", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &id,
		Action:       "billing.tariff.assign",
		ResourceType: "client_tariff",
		ResourceID:   &row.ID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"product": req.Product, "tariff_plan_id": req.TariffPlanID},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":             row.ID,
		"product":        row.Product,
		"tariff_plan_id": row.TariffPlanID,
		"price_override": billing.FormatMoneyPtr(row.PriceOverride),
	})
}

func (h *Handlers) UnassignClientTariff(w http.ResponseWriter, r *http.Request) {
	p, ok := requireAdmin(w, r)
	if !ok {
		return
	}
	if h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "billing unavailable")
		return
	}
	id, ok := pathUUID(w, r, "clientID")
	if !ok {
		return
	}
	product := sqlcdb.BillingProduct(chi.URLParam(r, "product"))
	if !billing.KnownProduct(product) {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid product")
		return
	}
	if !clientAssignable(w, r, h, id) {
		return
	}
	n, err := h.Store.Queries.DeleteClientTariff(r.Context(), sqlcdb.DeleteClientTariffParams{
		ClientID: id,
		Product:  product,
	})
	if err != nil {
		h.Log.Error("unassign tariff", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "tariff assignment not found")
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		ClientID:     &id,
		Action:       "billing.tariff.unassign",
		ResourceType: "client_tariff",
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"product": string(product)},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "product": product})
}

func clientAssignable(w http.ResponseWriter, r *http.Request, h *Handlers, id uuid.UUID) bool {
	cl, err := h.Store.Queries.GetClientByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "client not found")
			return false
		}
		h.Log.Error("load client for tariff", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return false
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "client not found")
		return false
	}
	return true
}

func ledgerRowJSON(id, clientID uuid.UUID, clientName string, txType sqlcdb.WalletTxType, amount decimal.Decimal, currency string, avail, held *decimal.Decimal, smsID *uuid.UUID, desc *string, at time.Time) map[string]any {
	out := map[string]any{
		"id":         id,
		"client_id":  clientID,
		"type":       txType,
		"amount":     billing.FormatMoney(amount),
		"currency":   currency,
		"created_at": at.UTC().Format(time.RFC3339),
	}
	if clientName != "" {
		out["client_name"] = clientName
	}
	if avail != nil {
		out["balance_after_available"] = billing.FormatMoney(*avail)
	}
	if held != nil {
		out["balance_after_held"] = billing.FormatMoney(*held)
	}
	if smsID != nil {
		out["sms_message_id"] = *smsID
	}
	if desc != nil {
		out["description"] = *desc
	}
	return out
}

func clientTariffJSON(t sqlcdb.ListClientTariffsRow) map[string]any {
	sell := t.PlanSellPrice
	if t.PriceOverride != nil {
		sell = *t.PriceOverride
	}
	return map[string]any{
		"id":              t.ID,
		"product":         t.Product,
		"tariff_plan_id":  t.TariffPlanID,
		"plan_code":       t.PlanCode,
		"plan_name":       t.PlanName,
		"sell_price":      billing.FormatMoney(sell),
		"plan_sell_price": billing.FormatMoney(t.PlanSellPrice),
		"price_override":  billing.FormatMoneyPtr(t.PriceOverride),
		"currency":        t.Currency,
		"is_active":       t.PlanIsActive,
	}
}

package client

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/messaging"
)

type sendRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.ClientID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if h.Messages == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "messaging unavailable")
		return
	}
	if !h.allowSend(w, r, *p.ClientID) {
		return
	}
	var req sendRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	msg, err := h.Messages.Enqueue(r.Context(), messaging.EnqueueInput{
		ClientID: *p.ClientID,
		From:     req.From,
		To:       req.To,
		Text:     req.Text,
	})
	if err != nil {
		writeMsgErr(w, h, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, MessageJSON(msg))
}

func (h *Handlers) GetMessage(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.ClientID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	msg, err := h.Messages.GetForClient(r.Context(), *p.ClientID, id)
	if err != nil {
		writeMsgErr(w, h, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, MessageJSON(msg))
}

func (h *Handlers) ListMessages(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.ClientID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	f := messaging.ListFilter{Limit: 50}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Limit = int32(n)
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			f.Offset = int32(n)
		}
	}
	if raw := r.URL.Query().Get("direction"); raw != "" {
		d := sqlcdb.SmsDirection(raw)
		if d != sqlcdb.SmsDirectionOutbound && d != sqlcdb.SmsDirectionInbound {
			httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid direction")
			return
		}
		f.Direction = &d
	}
	items, err := h.Messages.ListForClient(r.Context(), *p.ClientID, f)
	if err != nil {
		h.Log.Error("list messages", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, m := range items {
		out = append(out, MessageJSON(m))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handlers) allowSend(w http.ResponseWriter, r *http.Request, clientID uuid.UUID) bool {
	if h.Limiter == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return false
	}
	rate, burst := 5.0, 20.0
	if h.Settings != nil {
		if v, err := h.Settings.Get(r.Context()); err == nil && v.ClientRPSDefault > 0 {
			rate = v.ClientRPSDefault
			burst = rate * 2
			if burst < 20 {
				burst = 20
			}
		}
	}
	ok, retry, err := h.Limiter.AllowRate(r.Context(), "rl:http:client:"+clientID.String(), rate, burst)
	if err != nil {
		h.Log.Error("send rate limit", "err", err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return false
	}
	if !ok {
		if retry > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retry.Seconds())+1))
		}
		httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
		return false
	}
	return true
}

func writeMsgErr(w http.ResponseWriter, h *Handlers, err error) {
	WriteMessageError(w, h.Log, err)
}

func WriteMessageError(w http.ResponseWriter, log *slog.Logger, err error) {
	if httpx.WriteBillingError(w, err) {
		return
	}
	switch {
	case errors.Is(err, messaging.ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, messaging.ErrNotAssigned):
		httpx.WriteError(w, http.StatusForbidden, "not_assigned", "from number is not assigned to this client")
	case errors.Is(err, messaging.ErrInternationalOff):
		httpx.WriteError(w, http.StatusForbidden, "int_out_disabled", "international SMS is disabled")
	case errors.Is(err, messaging.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "message not found")
	default:
		if log != nil {
			log.Error("messaging", "err", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

func MessageJSON(m sqlcdb.SmsMessage) map[string]any {
	out := map[string]any{
		"id":         m.ID,
		"direction":  m.Direction,
		"from":       m.FromMsisdn,
		"to":         m.ToMsisdn,
		"text":       m.Text,
		"status":     m.Status,
		"created_at": m.CreatedAt.UTC().Format(time.RFC3339),
	}
	if m.ProviderSmsID != nil {
		out["provider_sms_id"] = *m.ProviderSmsID
	}
	if m.AcceptedAt != nil {
		out["accepted_at"] = m.AcceptedAt.UTC().Format(time.RFC3339)
	}
	if m.SentAt != nil {
		out["sent_at"] = m.SentAt.UTC().Format(time.RFC3339)
	}
	if m.DeliveredAt != nil {
		out["delivered_at"] = m.DeliveredAt.UTC().Format(time.RFC3339)
	}
	if m.FailedAt != nil {
		out["failed_at"] = m.FailedAt.UTC().Format(time.RFC3339)
	}
	if m.ProviderStatus != nil && *m.ProviderStatus != "" {
		out["provider_status"] = *m.ProviderStatus
	}
	if m.PduCount != nil {
		out["pdu_count"] = *m.PduCount
	}
	if m.BilledSegments != nil {
		out["billed_segments"] = *m.BilledSegments
	}
	if m.UnitSellPrice != nil && m.BilledSegments != nil {
		out["billed_amount"] = billing.FormatMoney(m.UnitSellPrice.Mul(decimal.NewFromInt(int64(*m.BilledSegments))))
		out["unit_sell_price"] = billing.FormatMoney(*m.UnitSellPrice)
	}
	if m.Currency != nil {
		out["currency"] = *m.Currency
	}
	if m.BillingAction.Valid {
		out["billing_action"] = m.BillingAction.BillingAction
	}
	return out
}

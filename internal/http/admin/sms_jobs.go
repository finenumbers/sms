package admin

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
)

func (h *Handlers) requireSMS(w http.ResponseWriter) bool {
	if h.Store == nil || h.Store.Queries == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "sms unavailable")
		return false
	}
	return true
}

func writeSMSInput(w http.ResponseWriter, err error) bool {
	var input smsInputError
	if errors.As(err, &input) {
		httpx.WriteError(w, http.StatusBadRequest, "validation", input.msg)
		return true
	}
	return false
}

func (h *Handlers) SMSListCampaigns(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	limit, offset, err := smsPage(r)
	if writeSMSInput(w, err) {
		return
	}
	clientID, err := parseOptionalUUID(r.URL.Query().Get("client_id"), "client_id")
	if writeSMSInput(w, err) {
		return
	}
	status, err := parseCampaignStatus(r.URL.Query().Get("status"))
	if writeSMSInput(w, err) {
		return
	}
	from, err := parseSMSMSISDN(r.URL.Query().Get("from"))
	if writeSMSInput(w, err) {
		return
	}
	var st sqlcdb.NullCampaignStatus
	if status != nil {
		st = sqlcdb.NullCampaignStatus{CampaignStatus: *status, Valid: true}
	}
	var fromPtr *string
	if from != "" {
		fromPtr = &from
	}
	rows, err := h.Store.Queries.AdminListCampaigns(r.Context(), sqlcdb.AdminListCampaignsParams{
		ClientID:   clientID,
		Status:     st,
		FromMsisdn: fromPtr,
		PageOffset: offset,
		PageLimit:  limit + 1,
	})
	if err != nil {
		h.Log.Error("admin list sms campaigns", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	page, more := trimPage(rows, int(limit))
	items := make([]map[string]any, 0, len(page))
	for _, row := range page {
		items = append(items, map[string]any{
			"id":              row.ID,
			"client_id":       row.ClientID,
			"client_name":     row.ClientName,
			"from":            row.FromMsisdn,
			"text_preview":    row.TextPreview,
			"status":          row.Status,
			"total_count":     row.TotalCount,
			"accepted_count":  row.AcceptedCount,
			"delivered_count": row.DeliveredCount,
			"failed_count":    row.FailedCount,
			"created_at":      row.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "has_more": more})
}

func (h *Handlers) SMSGetCampaign(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	row, err := h.Store.Queries.AdminGetCampaign(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	}
	if err != nil {
		h.Log.Error("admin get sms campaign", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := map[string]any{
		"id":              row.ID,
		"client_id":       row.ClientID,
		"from":            row.FromMsisdn,
		"text":            row.Text,
		"status":          row.Status,
		"total_count":     row.TotalCount,
		"accepted_count":  row.AcceptedCount,
		"delivered_count": row.DeliveredCount,
		"failed_count":    row.FailedCount,
		"created_at":      row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":      row.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"counters_note":   "Счётчики обновляет воркер пачкой и они могут отставать от живой очереди.",
	}
	if row.ClientName != nil {
		out["client_name"] = *row.ClientName
	}
	if row.CreatedBy != nil {
		out["created_by"] = *row.CreatedBy
	}
	if row.CreatedByEmail != nil {
		out["created_by_email"] = *row.CreatedByEmail
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) SMSCampaignSummary(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	if _, err := h.Store.Queries.AdminGetCampaign(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	} else if err != nil {
		h.Log.Error("admin sms campaign summary", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	st, err := h.Store.Queries.CampaignRecipientStats(r.Context(), id)
	if err != nil {
		h.Log.Error("admin sms campaign summary", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"total":    st.Total,
		"pending":  st.Pending,
		"enqueued": st.Enqueued,
		"skipped":  st.Skipped,
		"failed":   st.Failed,
	})
}

func (h *Handlers) SMSCampaignBilling(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	if _, err := h.Store.Queries.AdminGetCampaign(r.Context(), id); errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "campaign not found")
		return
	} else if err != nil {
		h.Log.Error("admin sms campaign billing", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	row, err := h.Store.Queries.AdminCampaignBilling(r.Context(), &id)
	if err != nil {
		h.Log.Error("admin sms campaign billing", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := map[string]any{
		"captured": formatMoneyText(row.Captured),
		"held":     formatMoneyText(row.Held),
		"released": formatMoneyText(row.Released),
	}
	if currency := strings.TrimSpace(asString(row.Currency)); currency != "" {
		out["currency"] = currency
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func (h *Handlers) SMSListRecipients(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "campaignID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	cursorAt, cursorID, err := decodeRecipientCursor(r.URL.Query().Get("cursor"))
	if writeSMSInput(w, err) {
		return
	}
	limit := smsRecipientLimit + 1
	var rows []sqlcdb.AdminListCampaignRecipientsRow
	if cursorAt.IsZero() {
		got, qerr := h.Store.Queries.AdminListCampaignRecipients(r.Context(), sqlcdb.AdminListCampaignRecipientsParams{
			CampaignID: id,
			PageLimit:  limit,
		})
		if qerr != nil {
			err = qerr
		} else {
			rows = got
		}
	} else {
		got, qerr := h.Store.Queries.AdminListCampaignRecipientsAfter(r.Context(), sqlcdb.AdminListCampaignRecipientsAfterParams{
			CampaignID:      id,
			CursorCreatedAt: cursorAt,
			CursorID:        cursorID,
			PageLimit:       limit,
		})
		if qerr != nil {
			err = qerr
		} else {
			rows = recipientRows(got)
		}
	}
	if err != nil {
		h.Log.Error("admin list sms recipients", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	page, more := trimPage(rows, int(smsRecipientLimit))
	items := make([]map[string]any, 0, len(page))
	for _, row := range page {
		items = append(items, recipientJSON(row))
	}
	out := map[string]any{"items": items, "has_more": more}
	if more && len(page) > 0 {
		last := page[len(page)-1]
		out["next_cursor"] = encodeRecipientCursor(last.CreatedAt, last.ID)
	}
	httpx.WriteJSON(w, http.StatusOK, out)
}

func recipientRows(rows []sqlcdb.AdminListCampaignRecipientsAfterRow) []sqlcdb.AdminListCampaignRecipientsRow {
	out := make([]sqlcdb.AdminListCampaignRecipientsRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, sqlcdb.AdminListCampaignRecipientsRow{
			ID:             row.ID,
			ToMsisdn:       row.ToMsisdn,
			Status:         row.Status,
			SmsMessageID:   row.SmsMessageID,
			CreatedAt:      row.CreatedAt,
			MessageStatus:  row.MessageStatus,
			ProviderSmsID:  row.ProviderSmsID,
			ProviderStatus: row.ProviderStatus,
			BilledSegments: row.BilledSegments,
			UnitSellPrice:  row.UnitSellPrice,
			Currency:       row.Currency,
			BillingAction:  row.BillingAction,
			LastError:      row.LastError,
		})
	}
	return out
}

func recipientJSON(row sqlcdb.AdminListCampaignRecipientsRow) map[string]any {
	item := map[string]any{
		"id":            row.ID,
		"to":            row.ToMsisdn,
		"status":        row.Status,
		"created_at":    row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"billing_state": billingState(row.BillingAction, row.UnitSellPrice),
	}
	if row.SmsMessageID != nil {
		item["message_id"] = *row.SmsMessageID
	}
	if row.MessageStatus.Valid {
		item["message_status"] = row.MessageStatus.SmsStatus
	}
	if row.ProviderSmsID != nil {
		item["provider_sms_id"] = *row.ProviderSmsID
	}
	if row.ProviderStatus != nil && *row.ProviderStatus != "" {
		item["provider_status"] = *row.ProviderStatus
	}
	if row.BilledSegments != nil {
		item["billed_segments"] = *row.BilledSegments
	}
	if amount := billedAmount(row.UnitSellPrice, row.BilledSegments); amount != nil {
		item["billed_amount"] = amount
	}
	if row.Currency != nil && strings.TrimSpace(*row.Currency) != "" {
		item["currency"] = strings.TrimSpace(*row.Currency)
	}
	if row.BillingAction.Valid {
		item["billing_action"] = row.BillingAction.BillingAction
	}
	if row.LastError != nil && *row.LastError != "" {
		item["last_error"] = *row.LastError
	}
	return item
}

func (h *Handlers) SMSListMessages(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	limit, offset, err := smsPage(r)
	if writeSMSInput(w, err) {
		return
	}
	direction, err := parseSMSDirection(r.URL.Query().Get("direction"))
	if writeSMSInput(w, err) {
		return
	}
	clientID, err := parseOptionalUUID(r.URL.Query().Get("client_id"), "client_id")
	if writeSMSInput(w, err) {
		return
	}
	status, err := parseSMSStatus(r.URL.Query().Get("status"))
	if writeSMSInput(w, err) {
		return
	}
	source, err := parseSMSSource(r.URL.Query().Get("source"))
	if writeSMSInput(w, err) {
		return
	}
	msisdn, err := parseSMSMSISDN(r.URL.Query().Get("msisdn"))
	if writeSMSInput(w, err) {
		return
	}
	providerID, err := parseProviderSMSID(r.URL.Query().Get("provider_sms_id"))
	if writeSMSInput(w, err) {
		return
	}
	filter := smsMessageFilter{
		Direction:  direction,
		ClientID:   clientID,
		Status:     status,
		Source:     source,
		MSISDN:     msisdn,
		ProviderID: providerID,
	}
	kind, err := chooseSMSListQuery(filter)
	if writeSMSInput(w, err) {
		return
	}
	rows, err := h.listSMSMessages(r, filter, kind, limit, offset)
	if err != nil {
		h.Log.Error("admin list sms messages", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	page, more := trimPage(rows, int(limit))
	items := make([]map[string]any, 0, len(page))
	for _, row := range page {
		items = append(items, messageListItem(row))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items, "has_more": more})
}

func (h *Handlers) listSMSMessages(r *http.Request, f smsMessageFilter, kind smsListQuery, limit, offset int32) ([]listedMessage, error) {
	q := h.Store.Queries
	ctx := r.Context()
	page := limit + 1
	var (
		rows any
		err  error
	)
	switch kind {
	case smsListOutbound:
		rows, err = q.AdminListOutboundMessages(ctx, sqlcdb.AdminListOutboundMessagesParams{PageOffset: offset, PageLimit: page})
	case smsListInbound:
		rows, err = q.AdminListInboundMessages(ctx, sqlcdb.AdminListInboundMessagesParams{PageOffset: offset, PageLimit: page})
	case smsListInboundStatus:
		rows, err = q.AdminListInboundMessagesByStatus(ctx, sqlcdb.AdminListInboundMessagesByStatusParams{Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListInflight:
		rows, err = q.AdminListOutboundInflight(ctx, sqlcdb.AdminListOutboundInflightParams{Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListFailed:
		rows, err = q.AdminListFailedMessages(ctx, sqlcdb.AdminListFailedMessagesParams{Direction: f.Direction, PageOffset: offset, PageLimit: page})
	case smsListDelivered:
		rows, err = q.AdminListDeliveredMessages(ctx, sqlcdb.AdminListDeliveredMessagesParams{Direction: f.Direction, PageOffset: offset, PageLimit: page})
	case smsListDirect:
		rows, err = q.AdminListDirectOutbound(ctx, sqlcdb.AdminListDirectOutboundParams{PageOffset: offset, PageLimit: page})
	case smsListDirectStatus:
		rows, err = q.AdminListDirectOutboundByStatus(ctx, sqlcdb.AdminListDirectOutboundByStatusParams{Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListCampaign:
		rows, err = q.AdminListCampaignOutbound(ctx, sqlcdb.AdminListCampaignOutboundParams{PageOffset: offset, PageLimit: page})
	case smsListCampaignInflight:
		rows, err = q.AdminListCampaignOutboundInflight(ctx, sqlcdb.AdminListCampaignOutboundInflightParams{Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListCampaignFailed:
		rows, err = q.AdminListCampaignOutboundFailed(ctx, sqlcdb.AdminListCampaignOutboundFailedParams{PageOffset: offset, PageLimit: page})
	case smsListCampaignStatus:
		rows, err = q.AdminListCampaignOutboundByStatus(ctx, sqlcdb.AdminListCampaignOutboundByStatusParams{Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListClient:
		rows, err = q.AdminListClientMessages(ctx, sqlcdb.AdminListClientMessagesParams{ClientID: f.ClientID, Direction: f.Direction, PageOffset: offset, PageLimit: page})
	case smsListClientStatus:
		rows, err = q.AdminListClientMessagesByStatus(ctx, sqlcdb.AdminListClientMessagesByStatusParams{ClientID: f.ClientID, Direction: f.Direction, Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListClientDirect:
		rows, err = q.AdminListClientDirect(ctx, sqlcdb.AdminListClientDirectParams{ClientID: f.ClientID, PageOffset: offset, PageLimit: page})
	case smsListClientDirectStatus:
		rows, err = q.AdminListClientDirectByStatus(ctx, sqlcdb.AdminListClientDirectByStatusParams{ClientID: f.ClientID, Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListClientCampaign:
		rows, err = q.AdminListClientCampaign(ctx, sqlcdb.AdminListClientCampaignParams{ClientID: f.ClientID, PageOffset: offset, PageLimit: page})
	case smsListClientCampaignStatus:
		rows, err = q.AdminListClientCampaignByStatus(ctx, sqlcdb.AdminListClientCampaignByStatusParams{ClientID: f.ClientID, Status: *f.Status, PageOffset: offset, PageLimit: page})
	case smsListPhone:
		var st sqlcdb.NullSmsStatus
		if f.Status != nil {
			st = sqlcdb.NullSmsStatus{SmsStatus: *f.Status, Valid: true}
		}
		var source *string
		if f.Source != "" {
			source = &f.Source
		}
		rows, err = q.AdminListMessagesByMSISDN(ctx, sqlcdb.AdminListMessagesByMSISDNParams{
			Msisdn:     f.MSISDN,
			Direction:  f.Direction,
			ClientID:   f.ClientID,
			Status:     st,
			Source:     source,
			PageOffset: offset,
			PageLimit:  page,
		})
	case smsListProvider:
		if offset > 0 {
			return nil, nil
		}
		row, qerr := q.AdminGetSmsMessageByProviderID(ctx, &f.ProviderID)
		if errors.Is(qerr, pgx.ErrNoRows) {
			return nil, nil
		}
		if qerr != nil {
			return nil, qerr
		}
		listed := listedFrom(row)
		if listed.Direction != f.Direction {
			return nil, nil
		}
		if f.ClientID != nil && (listed.ClientID == nil || *listed.ClientID != *f.ClientID) {
			return nil, nil
		}
		if f.Status != nil && listed.Status != *f.Status {
			return nil, nil
		}
		if f.Source == "direct" && listed.CampaignID != nil {
			return nil, nil
		}
		if f.Source == "campaign" && listed.CampaignID == nil {
			return nil, nil
		}
		return []listedMessage{listed}, nil
	default:
		return nil, smsInvalid("invalid status")
	}
	if err != nil {
		return nil, err
	}
	return smsRows(rows), nil
}

func (h *Handlers) SMSGetMessage(w http.ResponseWriter, r *http.Request) {
	if !h.requireSMS(w) {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "messageID"))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "invalid id")
		return
	}
	msg, err := h.Store.Queries.AdminGetSmsMessage(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "message not found")
		return
	}
	if err != nil {
		h.Log.Error("admin get sms message", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out := messageDetailJSON(msg)
	job, err := h.Store.Queries.AdminGetSendJobByMessage(r.Context(), id)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		h.Log.Error("admin get sms send job", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err == nil {
		out["send_job"] = sendJobJSON(job)
		attempts, aerr := h.Store.Queries.AdminListSendAttempts(r.Context(), job.ID)
		if aerr != nil {
			h.Log.Error("admin list sms attempts", "err", aerr)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		out["attempts"] = attemptJSON(attempts)
	}
	callbacks, err := h.Store.Queries.AdminListMessageCallbacks(r.Context(), &id)
	if err != nil {
		h.Log.Error("admin list sms callbacks", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	out["callbacks"] = callbackJSON(callbacks)
	httpx.WriteJSON(w, http.StatusOK, out)
}

func messageDetailJSON(m sqlcdb.AdminGetSmsMessageRow) map[string]any {
	out := map[string]any{
		"id":            m.ID,
		"direction":     m.Direction,
		"from":          m.FromMsisdn,
		"to":            m.ToMsisdn,
		"text":          m.Text,
		"status":        m.Status,
		"provider":      m.Provider,
		"created_at":    m.CreatedAt.UTC().Format(time.RFC3339Nano),
		"billing_state": billingState(m.BillingAction, m.UnitSellPrice),
		"attempts":      []any{},
		"callbacks":     []any{},
	}
	if m.ClientID != nil {
		out["client_id"] = *m.ClientID
	}
	if m.ClientName != nil {
		out["client_name"] = *m.ClientName
	}
	if m.ProviderSmsID != nil {
		out["provider_sms_id"] = *m.ProviderSmsID
	}
	if m.ProviderStatus != nil && *m.ProviderStatus != "" {
		out["provider_status"] = *m.ProviderStatus
	}
	if m.PduCount != nil {
		out["pdu_count"] = *m.PduCount
	}
	if m.CampaignID != nil {
		out["campaign_id"] = *m.CampaignID
	}
	if m.IdempotencyKey != nil {
		out["idempotency_key"] = *m.IdempotencyKey
	}
	putTime(out, "accepted_at", m.AcceptedAt)
	putTime(out, "sent_at", m.SentAt)
	putTime(out, "delivered_at", m.DeliveredAt)
	putTime(out, "failed_at", m.FailedAt)
	if m.UnitSellPrice != nil {
		out["unit_sell_price"] = billing.FormatMoney(*m.UnitSellPrice)
	}
	if m.BilledSegments != nil {
		out["billed_segments"] = *m.BilledSegments
	}
	if amount := billedAmount(m.UnitSellPrice, m.BilledSegments); amount != nil {
		out["billed_amount"] = amount
	}
	if m.TariffPlanID != nil {
		out["tariff_plan_id"] = *m.TariffPlanID
	}
	if m.TariffPlanCode != nil {
		out["tariff_plan_code"] = *m.TariffPlanCode
	}
	if m.Currency != nil && strings.TrimSpace(*m.Currency) != "" {
		out["currency"] = strings.TrimSpace(*m.Currency)
	}
	if m.BillingAction.Valid {
		out["billing_action"] = m.BillingAction.BillingAction
	}
	return out
}

func putTime(out map[string]any, key string, t *time.Time) {
	if t != nil {
		out[key] = t.UTC().Format(time.RFC3339Nano)
	}
}

func sendJobJSON(job sqlcdb.AdminGetSendJobByMessageRow) map[string]any {
	out := map[string]any{
		"id":           job.ID,
		"status":       job.Status,
		"attempt":      job.Attempt,
		"available_at": job.AvailableAt.UTC().Format(time.RFC3339Nano),
		"created_at":   job.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":   job.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
	putTime(out, "locked_at", job.LockedAt)
	if job.LockedBy != nil && *job.LockedBy != "" {
		out["locked_by"] = *job.LockedBy
	}
	if job.LastError != nil && *job.LastError != "" {
		out["last_error"] = *job.LastError
	}
	return out
}

func attemptJSON(rows []sqlcdb.AdminListSendAttemptsRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":         row.ID,
			"attempt":    row.Attempt,
			"created_at": row.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		if meta := rawJSON(row.RequestMeta); meta != nil {
			item["request_meta"] = meta
		}
		if row.HttpStatus != nil {
			item["http_status"] = *row.HttpStatus
		}
		if row.ResponseBody != nil && *row.ResponseBody != "" {
			item["response_body"] = *row.ResponseBody
		}
		if row.LatencyMs != nil {
			item["latency_ms"] = *row.LatencyMs
		}
		if row.ErrorKind.Valid {
			item["error_kind"] = row.ErrorKind.SendAttemptKind
		}
		out = append(out, item)
	}
	return out
}

func callbackJSON(rows []sqlcdb.AdminListMessageCallbacksRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":         row.ID,
			"kind":       row.Kind,
			"created_at": row.CreatedAt.UTC().Format(time.RFC3339Nano),
		}
		putTime(item, "processed_at", row.ProcessedAt)
		if parsed := rawJSON(row.Parsed); parsed != nil {
			item["parsed"] = parsed
		}
		out = append(out, item)
	}
	return out
}

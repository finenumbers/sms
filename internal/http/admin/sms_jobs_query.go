package admin

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const (
	smsMaxLimit       int32 = 100
	smsMaxOffset      int32 = 10000
	smsDefaultLimit   int32 = 50
	smsRecipientLimit int32 = 50
	smsTextPreview          = 80
)

type smsInputError struct{ msg string }

func (e smsInputError) Error() string { return e.msg }

func smsInvalid(msg string) error { return smsInputError{msg: msg} }

type smsListQuery string

const (
	smsListOutbound             smsListQuery = "outbound"
	smsListInbound              smsListQuery = "inbound"
	smsListInboundStatus        smsListQuery = "inbound_status"
	smsListInflight             smsListQuery = "inflight"
	smsListFailed               smsListQuery = "failed"
	smsListDelivered            smsListQuery = "delivered"
	smsListDirect               smsListQuery = "direct"
	smsListDirectStatus         smsListQuery = "direct_status"
	smsListCampaign             smsListQuery = "campaign"
	smsListCampaignInflight     smsListQuery = "campaign_inflight"
	smsListCampaignFailed       smsListQuery = "campaign_failed"
	smsListCampaignStatus       smsListQuery = "campaign_status"
	smsListClient               smsListQuery = "client"
	smsListClientStatus         smsListQuery = "client_status"
	smsListClientDirect         smsListQuery = "client_direct"
	smsListClientDirectStatus   smsListQuery = "client_direct_status"
	smsListClientCampaign       smsListQuery = "client_campaign"
	smsListClientCampaignStatus smsListQuery = "client_campaign_status"
	smsListPhone                smsListQuery = "phone"
	smsListProvider             smsListQuery = "provider"
)

type smsMessageFilter struct {
	Direction  sqlcdb.SmsDirection
	ClientID   *uuid.UUID
	Status     *sqlcdb.SmsStatus
	Source     string
	MSISDN     string
	ProviderID string
}

func smsPage(r *http.Request) (limit, offset int32, err error) {
	limit = smsDefaultLimit
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 1 {
			return 0, 0, smsInvalid("invalid limit")
		}
		limit = int32(n)
		if limit > smsMaxLimit {
			limit = smsMaxLimit
		}
	}
	if v := strings.TrimSpace(r.URL.Query().Get("offset")); v != "" {
		n, convErr := strconv.Atoi(v)
		if convErr != nil || n < 0 {
			return 0, 0, smsInvalid("invalid offset")
		}
		if n > int(smsMaxOffset) {
			return 0, 0, smsInvalid("offset too large")
		}
		offset = int32(n)
	}
	return limit, offset, nil
}

func parseSMSDirection(raw string) (sqlcdb.SmsDirection, error) {
	switch sqlcdb.SmsDirection(strings.TrimSpace(raw)) {
	case sqlcdb.SmsDirectionOutbound, sqlcdb.SmsDirectionInbound:
		return sqlcdb.SmsDirection(strings.TrimSpace(raw)), nil
	default:
		return "", smsInvalid("invalid direction")
	}
}

func parseSMSStatus(raw string) (*sqlcdb.SmsStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	st := sqlcdb.SmsStatus(raw)
	switch st {
	case sqlcdb.SmsStatusQueued, sqlcdb.SmsStatusAccepted, sqlcdb.SmsStatusSent, sqlcdb.SmsStatusDelivered, sqlcdb.SmsStatusFailed:
		return &st, nil
	default:
		return nil, smsInvalid("invalid status")
	}
}

func parseCampaignStatus(raw string) (*sqlcdb.CampaignStatus, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	st := sqlcdb.CampaignStatus(raw)
	switch st {
	case sqlcdb.CampaignStatusDraft, sqlcdb.CampaignStatusQueued, sqlcdb.CampaignStatusRunning,
		sqlcdb.CampaignStatusCompleted, sqlcdb.CampaignStatusFailed, sqlcdb.CampaignStatusCancelled:
		return &st, nil
	default:
		return nil, smsInvalid("invalid status")
	}
}

func parseSMSSource(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	switch raw {
	case "", "direct", "campaign":
		return raw, nil
	default:
		return "", smsInvalid("invalid source")
	}
}

func parseSMSMSISDN(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) < 8 || len(raw) > 15 {
		return "", smsInvalid("invalid msisdn")
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return "", smsInvalid("invalid msisdn")
		}
	}
	return raw, nil
}

func parseProviderSMSID(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) > 128 || strings.ContainsAny(raw, " \t\r\n%") {
		return "", smsInvalid("invalid provider_sms_id")
	}
	return raw, nil
}

func parseOptionalUUID(raw, field string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, smsInvalid("invalid " + field)
	}
	return &id, nil
}

func chooseSMSListQuery(f smsMessageFilter) (smsListQuery, error) {
	if f.ProviderID != "" && f.MSISDN != "" {
		return "", smsInvalid("use either msisdn or provider_sms_id")
	}
	if f.Direction != sqlcdb.SmsDirectionOutbound && f.Direction != sqlcdb.SmsDirectionInbound {
		return "", smsInvalid("invalid direction")
	}
	if f.Source != "" && f.Source != "direct" && f.Source != "campaign" {
		return "", smsInvalid("invalid source")
	}
	if f.Direction == sqlcdb.SmsDirectionInbound && f.Source != "" {
		return "", smsInvalid("source applies to outbound only")
	}
	if f.ProviderID != "" {
		return smsListProvider, nil
	}
	if f.MSISDN != "" {
		return smsListPhone, nil
	}
	if f.ClientID != nil {
		switch {
		case f.Source == "direct" && f.Status != nil:
			return smsListClientDirectStatus, nil
		case f.Source == "direct":
			return smsListClientDirect, nil
		case f.Source == "campaign" && f.Status != nil:
			return smsListClientCampaignStatus, nil
		case f.Source == "campaign":
			return smsListClientCampaign, nil
		case f.Status != nil:
			return smsListClientStatus, nil
		default:
			return smsListClient, nil
		}
	}
	switch {
	case f.Source == "direct" && f.Status != nil:
		return smsListDirectStatus, nil
	case f.Source == "direct":
		return smsListDirect, nil
	case f.Source == "campaign" && f.Status != nil && smsInflight(*f.Status):
		return smsListCampaignInflight, nil
	case f.Source == "campaign" && f.Status != nil && *f.Status == sqlcdb.SmsStatusFailed:
		return smsListCampaignFailed, nil
	case f.Source == "campaign" && f.Status != nil:
		return smsListCampaignStatus, nil
	case f.Source == "campaign":
		return smsListCampaign, nil
	case f.Status != nil && f.Direction == sqlcdb.SmsDirectionOutbound && smsInflight(*f.Status):
		return smsListInflight, nil
	case f.Status != nil && *f.Status == sqlcdb.SmsStatusFailed:
		return smsListFailed, nil
	case f.Status != nil && *f.Status == sqlcdb.SmsStatusDelivered:
		return smsListDelivered, nil
	case f.Status != nil && f.Direction == sqlcdb.SmsDirectionInbound:
		return smsListInboundStatus, nil
	case f.Direction == sqlcdb.SmsDirectionInbound:
		return smsListInbound, nil
	case f.Status != nil:
		return "", smsInvalid("invalid status")
	default:
		return smsListOutbound, nil
	}
}

func smsInflight(st sqlcdb.SmsStatus) bool {
	switch st {
	case sqlcdb.SmsStatusQueued, sqlcdb.SmsStatusAccepted, sqlcdb.SmsStatusSent:
		return true
	default:
		return false
	}
}

func smsMessageInflight(st string) bool {
	return smsInflight(sqlcdb.SmsStatus(st))
}

func smsCampaignInflight(st string) bool {
	switch sqlcdb.CampaignStatus(st) {
	case sqlcdb.CampaignStatusQueued, sqlcdb.CampaignStatusRunning:
		return true
	default:
		return false
	}
}

func billingState(action sqlcdb.NullBillingAction, price *decimal.Decimal) string {
	if price == nil {
		return "none"
	}
	if !action.Valid {
		return "hold"
	}
	return string(action.BillingAction)
}

func billedAmount(price *decimal.Decimal, segments *int32) any {
	if price == nil || segments == nil {
		return nil
	}
	return billing.FormatMoney(price.Mul(decimal.NewFromInt(int64(*segments))))
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

func formatMoneyText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return billing.FormatMoney(decimal.Zero)
	}
	d, err := decimal.NewFromString(raw)
	if err != nil {
		return raw
	}
	return billing.FormatMoney(d)
}

func trimPage[T any](rows []T, limit int) ([]T, bool) {
	if len(rows) > limit {
		return rows[:limit], true
	}
	return rows, false
}

type listedMessage struct {
	ID             uuid.UUID
	ClientID       *uuid.UUID
	ClientName     *string
	Direction      sqlcdb.SmsDirection
	FromMsisdn     string
	ToMsisdn       string
	TextPreview    string
	Status         sqlcdb.SmsStatus
	ProviderStatus *string
	PduCount       *int32
	BilledSegments *int32
	UnitSellPrice  *decimal.Decimal
	Currency       *string
	BillingAction  sqlcdb.NullBillingAction
	CampaignID     *uuid.UUID
	CreatedAt      time.Time
}

func listedFrom(row any) listedMessage {
	v := reflect.ValueOf(row)
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	return listedMessage{
		ID:             fieldAs[uuid.UUID](v, "ID"),
		ClientID:       fieldAs[*uuid.UUID](v, "ClientID"),
		ClientName:     fieldAs[*string](v, "ClientName"),
		Direction:      fieldAs[sqlcdb.SmsDirection](v, "Direction"),
		FromMsisdn:     fieldAs[string](v, "FromMsisdn"),
		ToMsisdn:       fieldAs[string](v, "ToMsisdn"),
		TextPreview:    fieldAs[string](v, "TextPreview"),
		Status:         fieldAs[sqlcdb.SmsStatus](v, "Status"),
		ProviderStatus: fieldAs[*string](v, "ProviderStatus"),
		PduCount:       fieldAs[*int32](v, "PduCount"),
		BilledSegments: fieldAs[*int32](v, "BilledSegments"),
		UnitSellPrice:  fieldAs[*decimal.Decimal](v, "UnitSellPrice"),
		Currency:       fieldAs[*string](v, "Currency"),
		BillingAction:  fieldAs[sqlcdb.NullBillingAction](v, "BillingAction"),
		CampaignID:     fieldAs[*uuid.UUID](v, "CampaignID"),
		CreatedAt:      fieldAs[time.Time](v, "CreatedAt"),
	}
}

func smsRows(rows any) []listedMessage {
	v := reflect.ValueOf(rows)
	if v.Kind() != reflect.Slice {
		panic("smsRows: expected slice")
	}
	out := make([]listedMessage, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		out = append(out, listedFrom(v.Index(i).Interface()))
	}
	return out
}

func fieldAs[T any](v reflect.Value, name string) T {
	f := v.FieldByName(name)
	if !f.IsValid() {
		panic("sms list row missing " + name)
	}
	val, ok := f.Interface().(T)
	if !ok {
		panic(fmt.Sprintf("sms list row field %s has type %s", name, f.Type()))
	}
	return val
}

func messageListItem(row listedMessage) map[string]any {
	item := map[string]any{
		"id":            row.ID,
		"direction":     row.Direction,
		"from":          row.FromMsisdn,
		"to":            row.ToMsisdn,
		"text_preview":  clipPreview(row.TextPreview),
		"status":        row.Status,
		"created_at":    row.CreatedAt.UTC().Format(time.RFC3339Nano),
		"billing_state": billingState(row.BillingAction, row.UnitSellPrice),
	}
	if row.ClientID != nil {
		item["client_id"] = *row.ClientID
	}
	if row.ClientName != nil && *row.ClientName != "" {
		item["client_name"] = *row.ClientName
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
	if row.Currency != nil && *row.Currency != "" {
		item["currency"] = strings.TrimSpace(*row.Currency)
	}
	if row.BillingAction.Valid {
		item["billing_action"] = row.BillingAction.BillingAction
	}
	if row.CampaignID != nil {
		item["campaign_id"] = *row.CampaignID
	}
	if _, ok := item["text"]; ok {
		delete(item, "text")
	}
	return item
}

func clipPreview(s string) string {
	if utf8.RuneCountInString(s) <= smsTextPreview+1 {
		return s
	}
	r := []rune(s)
	return string(r[:smsTextPreview]) + "…"
}

func encodeRecipientCursor(t time.Time, id uuid.UUID) string {
	raw := t.UTC().Format(time.RFC3339Nano) + "|" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeRecipientCursor(raw string) (time.Time, uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, uuid.UUID{}, nil
	}
	buf, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return time.Time{}, uuid.UUID{}, smsInvalid("invalid cursor")
	}
	ts, idRaw, ok := strings.Cut(string(buf), "|")
	if !ok {
		return time.Time{}, uuid.UUID{}, smsInvalid("invalid cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.UUID{}, smsInvalid("invalid cursor")
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return time.Time{}, uuid.UUID{}, smsInvalid("invalid cursor")
	}
	return t, id, nil
}

func rawJSON(b []byte) any {
	if len(b) == 0 || !json.Valid(b) {
		return nil
	}
	return json.RawMessage(append([]byte(nil), b...))
}

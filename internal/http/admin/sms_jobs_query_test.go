package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestSMSPageCaps(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?limit=500&offset=10", nil)
	limit, offset, err := smsPage(req)
	if err != nil || limit != smsMaxLimit || offset != 10 {
		t.Fatalf("limit=%d offset=%d err=%v", limit, offset, err)
	}
	req = httptest.NewRequest(http.MethodGet, "/?limit=0", nil)
	if _, _, err := smsPage(req); err == nil {
		t.Fatal("expected invalid limit")
	}
	req = httptest.NewRequest(http.MethodGet, "/?offset=10001", nil)
	if _, _, err := smsPage(req); err == nil {
		t.Fatal("expected offset cap")
	}
}

func TestParseSMSMSISDN(t *testing.T) {
	if _, err := parseSMSMSISDN("1234567"); err == nil {
		t.Fatal("short")
	}
	if _, err := parseSMSMSISDN("7900123abcd"); err == nil {
		t.Fatal("letters")
	}
	got, err := parseSMSMSISDN("79001234567")
	if err != nil || got != "79001234567" {
		t.Fatalf("got %s err %v", got, err)
	}
}

func TestChooseSMSListQuery(t *testing.T) {
	client := uuid.New()
	queued := sqlcdb.SmsStatusQueued
	failed := sqlcdb.SmsStatusFailed
	delivered := sqlcdb.SmsStatusDelivered
	cases := []struct {
		name string
		f    smsMessageFilter
		want smsListQuery
	}{
		{name: "phone and status stay on the phone query", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, MSISDN: "79001234567", Status: &failed}, want: smsListPhone},
		{name: "queued outbound uses inflight index query", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, Status: &queued}, want: smsListInflight},
		{name: "failed without client", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, Status: &failed}, want: smsListFailed},
		{name: "delivered without client", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, Status: &delivered}, want: smsListDelivered},
		{name: "client status does not use the global inflight query", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, ClientID: &client, Status: &queued}, want: smsListClientStatus},
		{name: "direct queued", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, Source: "direct", Status: &queued}, want: smsListDirectStatus},
		{name: "campaign queued", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, Source: "campaign", Status: &queued}, want: smsListCampaignInflight},
		{name: "campaign failed", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, Source: "campaign", Status: &failed}, want: smsListCampaignFailed},
		{name: "inbound queued", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionInbound, Status: &queued}, want: smsListInboundStatus},
		{name: "provider id", f: smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, ProviderID: "abc"}, want: smsListProvider},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := chooseSMSListQuery(tc.f)
			if err != nil || got != tc.want {
				t.Fatalf("got %s err %v", got, err)
			}
		})
	}
	if _, err := chooseSMSListQuery(smsMessageFilter{Direction: sqlcdb.SmsDirectionOutbound, MSISDN: "79001234567", ProviderID: "abc"}); err == nil {
		t.Fatal("phone and provider must not share one query")
	}
}

func TestMessageListItemOmitsBody(t *testing.T) {
	price := decimal.RequireFromString("3.5")
	segments := int32(2)
	item := messageListItem(listedMessage{
		ID:             uuid.New(),
		Direction:      sqlcdb.SmsDirectionOutbound,
		FromMsisdn:     "79001112233",
		ToMsisdn:       "79004445566",
		TextPreview:    "hello",
		Status:         sqlcdb.SmsStatusQueued,
		UnitSellPrice:  &price,
		BilledSegments: &segments,
		BillingAction:  sqlcdb.NullBillingAction{},
		CreatedAt:      time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
	})
	if _, ok := item["text"]; ok {
		t.Fatal("list item includes full text")
	}
	if item["text_preview"] != "hello" || item["has_more"] != nil {
		t.Fatalf("%v", item)
	}
	if item["billing_state"] != "hold" || item["billed_amount"] != "7.000000" {
		t.Fatalf("%v", item)
	}
	if _, ok := item["has_more"]; ok {
		t.Fatal("item should not carry page metadata")
	}
}

func TestBillingState(t *testing.T) {
	price := decimal.NewFromInt(1)
	if billingState(sqlcdb.NullBillingAction{}, nil) != "none" {
		t.Fatal("none")
	}
	if billingState(sqlcdb.NullBillingAction{}, &price) != "hold" {
		t.Fatal("hold")
	}
	if billingState(sqlcdb.NullBillingAction{BillingAction: sqlcdb.BillingActionCapture, Valid: true}, &price) != "capture" {
		t.Fatal("capture")
	}
	if billingState(sqlcdb.NullBillingAction{BillingAction: sqlcdb.BillingActionRelease, Valid: true}, &price) != "release" {
		t.Fatal("release")
	}
}

func TestRecipientCursorRoundTrip(t *testing.T) {
	id := uuid.New()
	at := time.Date(2026, 10, 8, 15, 4, 5, 123456000, time.UTC)
	gotAt, gotID, err := decodeRecipientCursor(encodeRecipientCursor(at, id))
	if err != nil || !gotAt.Equal(at) || gotID != id {
		t.Fatalf("at %s id %s err %v", gotAt, gotID, err)
	}
	if _, _, err := decodeRecipientCursor("%%%"); err == nil {
		t.Fatal("bad cursor")
	}
}

func TestListedFromGeneratedRow(t *testing.T) {
	row := sqlcdb.AdminListOutboundMessagesRow{
		ID:          uuid.New(),
		Direction:   sqlcdb.SmsDirectionOutbound,
		FromMsisdn:  "79001112233",
		ToMsisdn:    "79004445566",
		TextPreview: "hi",
		Status:      sqlcdb.SmsStatusQueued,
		CreatedAt:   time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC),
	}
	got := smsRows([]sqlcdb.AdminListOutboundMessagesRow{row})
	if len(got) != 1 || got[0].TextPreview != "hi" || got[0].FromMsisdn != row.FromMsisdn {
		t.Fatalf("%+v", got)
	}
}

func TestTrimPageHasMore(t *testing.T) {
	page, more := trimPage([]int{1, 2, 3}, 2)
	if !more || len(page) != 2 {
		t.Fatalf("%v %v", page, more)
	}
	page, more = trimPage([]int{1}, 2)
	if more || len(page) != 1 {
		t.Fatalf("%v %v", page, more)
	}
}

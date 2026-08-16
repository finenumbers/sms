package lookup

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestParseCSVPhones(t *testing.T) {
	got := ParseCSVPhones([]byte("phone\n+79001234567\n79001234568\n"))
	if len(got) != 2 || got[0] != "+79001234567" {
		t.Fatalf("got %#v", got)
	}
	got = ParseCSVPhones([]byte("телефон\n+79001112233\n"))
	if len(got) != 1 || got[0] != "+79001112233" {
		t.Fatalf("csv got %#v", got)
	}
}

func TestParseCheckType(t *testing.T) {
	ct, err := ParseCheckType("HLR")
	if err != nil || ct != sqlcdb.LookupCheckTypeHlr {
		t.Fatalf("hlr %v %v", ct, err)
	}
	ct, err = ParseCheckType("ping")
	if err != nil || ct != sqlcdb.LookupCheckTypePing {
		t.Fatalf("ping %v %v", ct, err)
	}
	if _, err := ParseCheckType("sms"); err == nil {
		t.Fatal("expected error")
	}
}

func TestWriteErrorConflict(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, wrap(ErrConflict, "conflict", "preview is being submitted"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "conflict") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestWriteErrorLookupDisabled(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(rec, wrap(ErrLookupDisabled, "lookup_disabled", "lookup is disabled"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "lookup_disabled") {
		t.Fatalf("body %s", rec.Body.String())
	}
}

func TestExportHeadersAndRow(t *testing.T) {
	headers := ExportHeaders(sqlcdb.LookupCheckTypeHlr)
	if len(headers) != 14 {
		t.Fatalf("hlr headers %d", len(headers))
	}
	if headers[0] != "Номер" || headers[6] != "MCC" || headers[7] != "MNC" || headers[13] != "Ошибка" {
		t.Fatalf("hlr headers %#v", headers)
	}
	pingHeaders := ExportHeaders(sqlcdb.LookupCheckTypePing)
	if len(pingHeaders) != 4 || pingHeaders[0] != "Номер" || pingHeaders[3] != "Ошибка" {
		t.Fatalf("ping headers %#v", pingHeaders)
	}
	reachable := true
	item := sqlcdb.LookupItem{
		PhoneE164:    "+79001234567",
		Status:       sqlcdb.LookupItemStatusCompleted,
		ResultStatus: strPtr("reachable"),
		IsReachable:  &reachable,
	}
	row := ExportRow(sqlcdb.LookupCheckTypePing, item)
	if len(row) != 4 || row[0] != "+79001234567" || row[1] != "готово" || row[2] != "в сети" || row[3] != "—" {
		t.Fatalf("row %#v", row)
	}
	mcc, mnc := "250", "01"
	hlrItem := item
	hlrItem.Mcc = &mcc
	hlrItem.Mnc = &mnc
	hlrRow := ExportRow(sqlcdb.LookupCheckTypeHlr, hlrItem)
	if len(hlrRow) != 14 || hlrRow[6] != "250" || hlrRow[7] != "01" || hlrRow[10] != "—" {
		t.Fatalf("hlr row %#v", hlrRow)
	}
	noResult := sqlcdb.LookupItem{PhoneE164: "+79000000000", IsReachable: &reachable}
	if ExportRow(sqlcdb.LookupCheckTypePing, noResult)[2] != "да" {
		t.Fatalf("result fallback %#v", ExportRow(sqlcdb.LookupCheckTypePing, noResult))
	}
	raw, err := BuildXLSX(sqlcdb.LookupCheckTypePing, []sqlcdb.LookupItem{item})
	if err != nil || len(raw) < 100 {
		t.Fatalf("xlsx %v %d", err, len(raw))
	}
}

func TestPreviewJSONStats(t *testing.T) {
	row := sqlcdb.LookupCsvPreview{
		PhoneCount: 2,
		PhonesJson: []byte(`{"phones":["+79001111111","+79002222222"],"row_count":4,"invalid_count":0,"duplicate_count":2}`),
	}
	got := PreviewJSON(row)
	if got["row_count"] != 4 || got["duplicate_count"] != 2 || got["invalid_count"] != 0 || got["phone_count"] != int32(2) {
		t.Fatalf("%#v", got)
	}
}

func TestPagePreviewPhones(t *testing.T) {
	phones := []string{"a", "b", "c"}
	items, total := pagePreviewPhones(phones, 2, 0)
	if total != 3 || len(items) != 2 || items[0]["line"] != 1 || items[1]["phone"] != "b" {
		t.Fatalf("%v total=%d", items, total)
	}
	items, total = pagePreviewPhones(phones, 2, 2)
	if total != 3 || len(items) != 1 || items[0]["line"] != 3 || items[0]["phone"] != "c" {
		t.Fatalf("tail %#v", items)
	}
	items, _ = pagePreviewPhones(phones, 10, 10)
	if len(items) != 0 {
		t.Fatalf("past end %#v", items)
	}
}

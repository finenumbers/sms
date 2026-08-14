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
	if len(ExportHeaders(sqlcdb.LookupCheckTypeHlr)) != 15 {
		t.Fatalf("hlr headers %d", len(ExportHeaders(sqlcdb.LookupCheckTypeHlr)))
	}
	if len(ExportHeaders(sqlcdb.LookupCheckTypePing)) != 5 {
		t.Fatalf("ping headers %d", len(ExportHeaders(sqlcdb.LookupCheckTypePing)))
	}
	reachable := true
	item := sqlcdb.LookupItem{
		PhoneE164:    "+79001234567",
		Status:       sqlcdb.LookupItemStatusCompleted,
		ResultStatus: strPtr("reachable"),
		IsReachable:  &reachable,
	}
	row := ExportRow(sqlcdb.LookupCheckTypePing, item)
	if row[0] != "+79001234567" || row[2] != "в сети" || row[3] != "да" {
		t.Fatalf("row %#v", row)
	}
	raw, err := BuildXLSX(sqlcdb.LookupCheckTypePing, []sqlcdb.LookupItem{item})
	if err != nil || len(raw) < 100 {
		t.Fatalf("xlsx %v %d", err, len(raw))
	}
}

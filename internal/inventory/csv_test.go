package inventory

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestParseNumberFileUTF8AndBOM(t *testing.T) {
	raw := []byte("\ufeffmsisdn,region,notes\n+7 (999) 111-22-33,Москва,test\n89992223344\n")
	got := ParseNumberFile(raw)
	if got.Encoding != "utf-8" {
		t.Fatalf("encoding=%s", got.Encoding)
	}
	if len(got.Invalid) != 0 {
		t.Fatalf("invalid=%+v", got.Invalid)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("rows=%d", len(got.Rows))
	}
	if got.Rows[0].MSISDN != "79991112233" || got.Rows[0].Region != "Москва" {
		t.Fatalf("row0=%+v", got.Rows[0])
	}
	if got.Rows[1].MSISDN != "79992223344" {
		t.Fatalf("row1=%+v", got.Rows[1])
	}
}

func TestParseNumberFileWindows1251(t *testing.T) {
	plain := "номер;регион\n79991112233;Москва\n"
	enc := charmap.Windows1251.NewEncoder()
	raw, err := enc.Bytes([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	got := ParseNumberFile(raw)
	if got.Encoding != "windows-1251" {
		t.Fatalf("encoding=%s", got.Encoding)
	}
	if len(got.Rows) != 1 || got.Rows[0].MSISDN != "79991112233" {
		t.Fatalf("rows=%+v invalid=%+v", got.Rows, got.Invalid)
	}
	if got.Rows[0].Region != "Москва" {
		t.Fatalf("region=%q", got.Rows[0].Region)
	}
}

func TestParseNumberFileInvalidAndComment(t *testing.T) {
	raw := []byte("# comment\n\nabc\n79991112233\n")
	got := ParseNumberFile(raw)
	if len(got.Rows) != 1 || got.Rows[0].MSISDN != "79991112233" {
		t.Fatalf("rows=%+v", got.Rows)
	}
	if len(got.Invalid) != 1 || !strings.Contains(got.Invalid[0].Error, "7XXXXXXXXXX") {
		t.Fatalf("invalid=%+v", got.Invalid)
	}
}

func TestParseHeaderDoesNotStealRegion(t *testing.T) {
	raw := []byte("msisdn\n79991112233\n")
	got := ParseNumberFile(raw)
	if len(got.Rows) != 1 {
		t.Fatalf("rows=%+v invalid=%+v", got.Rows, got.Invalid)
	}
	if got.Rows[0].Region != "" {
		t.Fatalf("region leaked: %+v", got.Rows[0])
	}
}

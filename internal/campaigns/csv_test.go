package campaigns

import (
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"golang.org/x/text/encoding/charmap"
)

func TestParseRecipientFileUTF8AndBOM(t *testing.T) {
	raw := []byte("\ufeffmsisdn\n+7 (999) 111-22-33\n14155551234\n79991112233\n")
	got := ParseRecipientFile(raw)
	if got.Encoding != "utf-8" {
		t.Fatalf("encoding=%s", got.Encoding)
	}
	if len(got.Invalid) != 0 {
		t.Fatalf("invalid=%+v", got.Invalid)
	}
	if len(got.MSISDNs) != 2 {
		t.Fatalf("msisdns=%v (dup 79991112233 should collapse)", got.MSISDNs)
	}
	if got.MSISDNs[0] != "79991112233" || got.MSISDNs[1] != "14155551234" {
		t.Fatalf("msisdns=%v", got.MSISDNs)
	}
}

func TestParseRecipientFileWindows1251(t *testing.T) {
	plain := "номер\n79991112233\n"
	enc := charmap.Windows1251.NewEncoder()
	raw, err := enc.Bytes([]byte(plain))
	if err != nil {
		t.Fatal(err)
	}
	got := ParseRecipientFile(raw)
	if got.Encoding != "windows-1251" {
		t.Fatalf("encoding=%s", got.Encoding)
	}
	if len(got.MSISDNs) != 1 || got.MSISDNs[0] != "79991112233" {
		t.Fatalf("got=%+v", got)
	}
}

func TestParseRecipientFileInvalid(t *testing.T) {
	got := ParseRecipientFile([]byte("# x\n\nabc\n79992223344\n"))
	if len(got.MSISDNs) != 1 || got.MSISDNs[0] != "79992223344" {
		t.Fatalf("msisdns=%v", got.MSISDNs)
	}
	if len(got.Invalid) != 1 || got.Invalid[0].Value != "abc" {
		t.Fatalf("invalid=%+v", got.Invalid)
	}
}

func TestNormalizeRecipientListDedupe(t *testing.T) {
	got, inv := NormalizeRecipientList([]string{"8 999 111 22 33", "79991112233", "00"})
	if len(got) != 1 || got[0] != "79991112233" {
		t.Fatalf("got=%v", got)
	}
	if len(inv) != 1 {
		t.Fatalf("invalid=%+v", inv)
	}
}

func TestFrozenAfterDraft(t *testing.T) {
	if Frozen(sqlcdb.CampaignStatusDraft) {
		t.Fatal("draft is mutable")
	}
	for _, st := range []sqlcdb.CampaignStatus{
		sqlcdb.CampaignStatusQueued,
		sqlcdb.CampaignStatusRunning,
		sqlcdb.CampaignStatusCompleted,
		sqlcdb.CampaignStatusCancelled,
		sqlcdb.CampaignStatusFailed,
	} {
		if !Frozen(st) {
			t.Fatalf("%s should be frozen", st)
		}
	}
}

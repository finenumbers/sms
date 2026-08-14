package lookup

import (
	"testing"
	"time"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

func TestMergeNormalizedKeepsExistingFields(t *testing.T) {
	imsi := "250011234567890"
	item := sqlcdb.LookupItem{
		PhoneE164:        "+79991234567",
		Imsi:             &imsi,
		NormalizedResult: []byte(`{"extras":{"msc":"79001234567"}}`),
	}
	n := smsc.NormalizedResult{
		PhoneE164: "+79991234567",
		Extras:    map[string]any{"region": "Moscow"},
	}
	got := mergeNormalizedWithItem(n, item)
	if got.IMSI != imsi {
		t.Fatalf("imsi %s", got.IMSI)
	}
	if got.Extras["msc"] != "79001234567" || got.Extras["region"] != "Moscow" {
		t.Fatalf("extras %#v", got.Extras)
	}
}

func TestNeedsHLREnrich(t *testing.T) {
	n := smsc.NormalizedResult{CheckType: smsc.CheckHLR, Extras: map[string]any{}}
	if !needsHLREnrich(n, sqlcdb.LookupCheckTypeHlr) {
		t.Fatal("missing imsi/msc")
	}
	n.IMSI = "25001"
	n.Extras["msc"] = "7900"
	if needsHLREnrich(n, sqlcdb.LookupCheckTypeHlr) {
		t.Fatal("complete hlr should not enrich")
	}
	ping := smsc.NormalizedResult{CheckType: smsc.CheckPing, Extras: map[string]any{}}
	if needsHLREnrich(ping, sqlcdb.LookupCheckTypePing) {
		t.Fatal("ping should not enrich")
	}
}

func TestPreferResultStatusDoesNotRegress(t *testing.T) {
	if preferResultStatus("pending", "reachable") != "reachable" {
		t.Fatal("must keep terminal")
	}
	if preferResultStatus("unreachable", "reachable") != "unreachable" {
		t.Fatal("incoming terminal wins")
	}
}

func TestHLRFieldsImproved(t *testing.T) {
	item := sqlcdb.LookupItem{NormalizedResult: []byte(`{}`)}
	n := smsc.NormalizedResult{IMSI: "25001", Extras: map[string]any{}}
	if !hlrFieldsImproved(n, item) {
		t.Fatal("new imsi")
	}
	imsi := "25001"
	item.Imsi = &imsi
	if hlrFieldsImproved(smsc.NormalizedResult{IMSI: "25001", Extras: map[string]any{}}, item) {
		t.Fatal("same imsi is not an improvement")
	}
}

func TestBackoff(t *testing.T) {
	if backoff(30, 1) != 30*time.Second {
		t.Fatalf("%s", backoff(30, 1))
	}
	if backoff(30, 3) != 120*time.Second {
		t.Fatalf("%s", backoff(30, 3))
	}
}

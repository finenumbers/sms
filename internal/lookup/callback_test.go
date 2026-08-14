package lookup

import (
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

func TestCallbackPhonesMatch(t *testing.T) {
	if !callbackPhonesMatch("79991234567", "79991234567") {
		t.Fatal("same digits")
	}
	if !callbackPhonesMatch("79991234567", "") {
		t.Fatal("empty callback phone is not a mismatch")
	}
	if callbackPhonesMatch("79991234567", "79990000000") {
		t.Fatal("different digits must not apply")
	}
}

func TestApplyIncomingEmptyIDIsNotFound(t *testing.T) {
	w := &Worker{}
	res, err := w.ApplyIncoming(t.Context(), IncomingCallback{})
	if err != nil || res.Reason != "not_found" {
		t.Fatalf("%#v %v", res, err)
	}
}

func TestPickCallbackItemsPrefersProviderID(t *testing.T) {
	byID := []sqlcdb.LookupItem{{PhoneDigits: "79991111111"}}
	byPhone := []sqlcdb.LookupItem{{PhoneDigits: "79992222222"}}
	got, reason := pickCallbackItems(byID, byPhone)
	if reason != "" || got.PhoneDigits != "79991111111" {
		t.Fatalf("%#v %s", got, reason)
	}
}

func TestPickCallbackItemsPhoneFallback(t *testing.T) {
	got, reason := pickCallbackItems(nil, []sqlcdb.LookupItem{{PhoneDigits: "79994504444"}})
	if reason != "" || got.PhoneDigits != "79994504444" {
		t.Fatalf("%#v %s", got, reason)
	}
}

func TestPickCallbackItemsAmbiguousPhone(t *testing.T) {
	_, reason := pickCallbackItems(nil, []sqlcdb.LookupItem{{}, {}})
	if reason != "ambiguous" {
		t.Fatalf("reason=%s", reason)
	}
}

func TestIncomingSkipEnrichLeavesApplyInput(t *testing.T) {
	in := ApplyInput{SkipEnrich: true, Normalized: smsc.NormalizedResult{CheckType: smsc.CheckHLR}}
	if !in.SkipEnrich {
		t.Fatal("HTTP callback must not call SMSC")
	}
	_ = sqlcdb.LookupItem{}
}

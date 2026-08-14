package lookup

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

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

func TestCallbackPhoneDigitsNormalizesRU(t *testing.T) {
	if got := CallbackPhoneDigits("89139447008"); got != "79139447008" {
		t.Fatalf("8→7: %s", got)
	}
	if got := CallbackPhoneDigits("+89139447008"); got != "79139447008" {
		t.Fatalf("+8→7: %s", got)
	}
	if got := CallbackPhoneDigits("9139447008"); got != "79139447008" {
		t.Fatalf("10-digit 9: %s", got)
	}
	if got := CallbackPhoneDigits("+7 913 944-70-08"); got != "79139447008" {
		t.Fatalf("spaces: %s", got)
	}
	if got := CallbackPhoneDigits("79139447008"); got != "79139447008" {
		t.Fatalf("digits: %s", got)
	}
	if CallbackPhoneDigits("") != "" {
		t.Fatal("empty")
	}
}

func TestCallbackPhonesMatchLast10(t *testing.T) {
	if !callbackPhonesMatch("79607977373", "9607977373") {
		t.Fatal("last-10 must match RU mobile without country code")
	}
	if callbackPhonesMatch("79607977373", "9607977374") {
		t.Fatal("different tail")
	}
}

func TestClientSendIDMatchesSubmitKey(t *testing.T) {
	itemID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	got := ClientSendID(sqlcdb.LookupCheckTypeHlr, itemID)
	want := strconv.Itoa(smsc.ClientIDFromKey(smsc.SendIdempotencyKey(smsc.CheckHLR, itemID.String())))
	if got != want || got == "" || got == "0" {
		t.Fatalf("client send id=%s want=%s", got, want)
	}
	if ClientSendID(sqlcdb.LookupCheckTypePing, itemID) == got {
		t.Fatal("ping and hlr must hash differently")
	}
}

func TestPickCallbackItemsTwoPhonesStayAmbiguous(t *testing.T) {
	_, reason := pickCallbackItems(nil, []sqlcdb.LookupItem{
		{PhoneDigits: "79994504444"},
		{PhoneDigits: "79994504444"},
	})
	if reason != "ambiguous" {
		t.Fatalf("must not pick newest, reason=%s", reason)
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

func TestShouldConcludeCallback(t *testing.T) {
	if ShouldConcludeCallback(IncomingResult{Reason: "not_found"}) {
		t.Fatal("not_found must stay unprocessed")
	}
	if ShouldConcludeCallback(IncomingResult{Reason: "ambiguous"}) {
		t.Fatal("ambiguous must stay unprocessed")
	}
	if ShouldConcludeCallback(IncomingResult{}) {
		t.Fatal("empty reason must stay unprocessed")
	}
	if !ShouldConcludeCallback(IncomingResult{Applied: true}) {
		t.Fatal("applied")
	}
	if !ShouldConcludeCallback(IncomingResult{Duplicate: true}) {
		t.Fatal("duplicate")
	}
}

func TestShouldMarkStoredCallback(t *testing.T) {
	fresh := 10 * time.Minute
	old := callbackNotFoundTTL + time.Second
	if shouldMarkStoredCallback(IncomingResult{Reason: "item_not_found"}, fresh) {
		t.Fatal("fresh item_not_found waits")
	}
	if shouldMarkStoredCallback(IncomingResult{Reason: "not_found"}, fresh) {
		t.Fatal("fresh not_found waits")
	}
	if !shouldMarkStoredCallback(IncomingResult{Reason: "not_found"}, old) {
		t.Fatal("stale not_found is abandoned")
	}
	if shouldMarkStoredCallback(IncomingResult{Reason: "ambiguous"}, old) {
		t.Fatal("ambiguous never expires: wait until one item gets provider id")
	}
	if shouldMarkStoredCallback(IncomingResult{Reason: ""}, fresh) {
		t.Fatal("fresh empty reason waits")
	}
	if !shouldMarkStoredCallback(IncomingResult{Reason: ""}, old) {
		t.Fatal("stale empty reason is abandoned")
	}
	if !shouldMarkStoredCallback(IncomingResult{Applied: true}, fresh) {
		t.Fatal("applied is marked")
	}
}

func TestIncomingFromStoredSkipsEnrichAndPrefersNormalizedPhone(t *testing.T) {
	id := "1001"
	in := incomingFromStored(sqlcdb.ProviderLookupCallback{
		ProviderMessageID: &id,
		NormalizedResult:  []byte(`{"phone_e164":"+79139447008","lifecycle_status":"completed"}`),
		RawPayload:        []byte(`{"phone":89139447008,"id":1001}`),
	})
	if !in.SkipEnrich {
		t.Fatal("replay must not call SMSC")
	}
	if in.PhoneDigits != "79139447008" {
		t.Fatalf("phone=%s", in.PhoneDigits)
	}
	if in.ProviderMessageID != "1001" {
		t.Fatalf("id=%s", in.ProviderMessageID)
	}
	if in.Normalized.LifecycleStatus != smsc.LifecycleCompleted {
		t.Fatalf("lifecycle=%s", in.Normalized.LifecycleStatus)
	}
}

func TestIncomingFromStoredPhonesField(t *testing.T) {
	in := incomingFromStored(sqlcdb.ProviderLookupCallback{
		RawPayload: []byte(`{"phones":"+89607977373","id":99}`),
	})
	if in.PhoneDigits != "79607977373" {
		t.Fatalf("phones +8: %s", in.PhoneDigits)
	}
	if in.ProviderMessageID != "99" {
		t.Fatalf("id=%s", in.ProviderMessageID)
	}
}

func TestIncomingFromStoredNumericRawPhone(t *testing.T) {
	in := incomingFromStored(sqlcdb.ProviderLookupCallback{
		RawPayload: []byte(`{"phone":89139447008,"id":42}`),
	})
	if in.PhoneDigits != "79139447008" {
		t.Fatalf("numeric JSON phone: %s", in.PhoneDigits)
	}
	if in.ProviderMessageID != "42" {
		t.Fatalf("numeric JSON id: %s", in.ProviderMessageID)
	}
	if !in.SkipEnrich {
		t.Fatal("SkipEnrich")
	}
}

func TestIncomingSkipEnrichLeavesApplyInput(t *testing.T) {
	in := ApplyInput{SkipEnrich: true, Normalized: smsc.NormalizedResult{CheckType: smsc.CheckHLR}}
	if !in.SkipEnrich {
		t.Fatal("HTTP callback must not call SMSC")
	}
	_ = sqlcdb.LookupItem{}
}

func TestAsStringAnyJSONNumber(t *testing.T) {
	if got := asStringAny(float64(89139447008)); got != "89139447008" {
		t.Fatalf("float64: %s", got)
	}
	if got := asStringAny(json.Number("1001")); got != "1001" {
		t.Fatalf("json.Number: %s", got)
	}
}

func TestDrainCallbacksNoopsWithoutStore(t *testing.T) {
	n, err := (&Worker{}).DrainCallbacks(t.Context())
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestApplyStoredCallbacksHasNoDeadline(t *testing.T) {
	n, err := (&Worker{}).applyStoredCallbacks(t.Context())
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

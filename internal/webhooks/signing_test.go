package webhooks

import (
	"strings"
	"testing"
	"time"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestSignAndVerify(t *testing.T) {
	secret := "whsec_test"
	body := `{"api_version":"v1","id":"del_1","type":"check.completed"}`
	ts := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC).Unix()
	header, sig := Sign(secret, body, ts)
	if header != "t=1786708800,v1="+sig && !strings.Contains(header, "v1="+sig) {
		t.Fatalf("header %s", header)
	}
	if !Verify(secret, body, header, ts, 300) {
		t.Fatal("verify same timestamp")
	}
	if Verify(secret, body, header, ts+400, 300) {
		t.Fatal("stale signature accepted")
	}
	if Verify("other", body, header, ts, 300) {
		t.Fatal("wrong secret accepted")
	}
	if Verify(secret, `{}`, header, ts, 300) {
		t.Fatal("mutated body accepted")
	}
}

func TestSubscribesEmptyMeansAll(t *testing.T) {
	if !Subscribes(nil, EventCheckCompleted) || !Subscribes([]string{}, EventJobCompleted) {
		t.Fatal("empty events must match all known")
	}
	if Subscribes([]string{EventCheckFailed}, EventCheckCompleted) {
		t.Fatal("filtered endpoint matched other event")
	}
	if Subscribes([]string{EventCheckFailed}, EventCheckFailed) != true {
		t.Fatal("exact event")
	}
}

func TestRetryDelayAndNext(t *testing.T) {
	if RetryDelay(1) != 30*time.Second {
		t.Fatalf("attempt 1 %s", RetryDelay(1))
	}
	if RetryDelay(2) != 60*time.Second {
		t.Fatalf("attempt 2 %s", RetryDelay(2))
	}
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	if NextAttemptAt(8, 8, now) != nil {
		t.Fatal("max attempts must stop")
	}
	next := NextAttemptAt(1, 8, now)
	if next == nil || !next.Equal(now.Add(30*time.Second)) {
		t.Fatalf("next %#v", next)
	}
}

func TestCheckPayloadSnakeCase(t *testing.T) {
	item := sqlcdb.LookupItem{
		CheckType: sqlcdb.LookupCheckTypeHlr,
		Status:    sqlcdb.LookupItemStatusCompleted,
		PhoneE164: "+79001234567",
	}
	data := CheckData(item)
	if data["phone"] != "+79001234567" || data["type"] != "hlr" {
		t.Fatalf("%#v", data)
	}
	env := Envelope("d1", EventCheckCompleted, time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC), data)
	raw, err := Serialize(env)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, camel := range []string{"apiVersion", "createdAt", "jobId", "phoneE164", "checkType"} {
		if strings.Contains(s, camel) {
			t.Fatalf("camelCase leaked %s in %s", camel, s)
		}
	}
	if !strings.Contains(s, `"api_version":"v1"`) || !strings.Contains(s, `"created_at"`) {
		t.Fatalf("envelope %s", s)
	}
}

func TestEventForItemStatus(t *testing.T) {
	if EventForItemStatus("completed") != EventCheckCompleted {
		t.Fatal("completed")
	}
	if EventForItemStatus("failed") != EventCheckFailed {
		t.Fatal("failed")
	}
}

func TestNormalizeEvents(t *testing.T) {
	got, err := normalizeEvents(nil)
	if err != nil || len(got) != 0 {
		t.Fatalf("empty %#v %v", got, err)
	}
	if _, err := normalizeEvents([]string{"nope"}); err == nil {
		t.Fatal("unknown")
	}
	got, err = normalizeEvents([]string{EventCheckCompleted, EventCheckCompleted})
	if err != nil || len(got) != 1 {
		t.Fatalf("dedupe %#v %v", got, err)
	}
}

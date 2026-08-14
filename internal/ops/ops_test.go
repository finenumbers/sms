package ops

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRedactJSONNestedMessageAndTokens(t *testing.T) {
	in := []byte(`{
		"password": "secret-pw",
		"token": "access-tok",
		"refresh_token": "ref",
		"data": [{"message": "hello SMS", "sms_id": "abc", "sent": true}],
		"email": "a@b.c"
	}`)
	got := RedactJSON(in)
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["password"] != "[redacted]" || m["token"] != "[redacted]" || m["refresh_token"] != "[redacted]" {
		t.Fatalf("tokens: %s", got)
	}
	if m["email"] != "a@b.c" {
		t.Fatalf("email: %s", got)
	}
	data, _ := m["data"].([]any)
	row, _ := data[0].(map[string]any)
	if row["message"] != "[redacted]" {
		t.Fatalf("nested message: %s", got)
	}
	if row["sms_id"] != "abc" {
		t.Fatalf("sms_id: %s", got)
	}
}

func TestRedactJSONKeepsProviderEnvelopeMessage(t *testing.T) {
	in := []byte(`{"success":false,"code":500,"message":"an unexpected error has occurred","request_id":"req-1"}`)
	got := RedactJSON(in)
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["message"] != "an unexpected error has occurred" {
		t.Fatalf("envelope message redacted: %s", got)
	}
	if m["request_id"] != "req-1" {
		t.Fatalf("request_id: %s", got)
	}
}

func TestRedactJSONSendText(t *testing.T) {
	in := []byte(`{"from_number":"79391125968","to_number":"79994504444","text":"secret sms"}`)
	got := RedactJSON(in)
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["text"] != "[redacted]" {
		t.Fatalf("text: %s", got)
	}
	if m["from_number"] != "79391125968" {
		t.Fatalf("from: %s", got)
	}
}

func TestTruncateJSONAlwaysValid(t *testing.T) {
	big := []byte(`{"data":"` + strings.Repeat("x", 200) + `"}`)
	got := truncateJSON(big, 80)
	if !json.Valid(got) {
		t.Fatalf("invalid json: %s", got)
	}
	var m map[string]any
	if err := json.Unmarshal(got, &m); err != nil {
		t.Fatal(err)
	}
	if m["_truncated"] != true {
		t.Fatalf("%s", got)
	}
	if truncateJSON([]byte(`{"ok":true}`), 80) == nil || !json.Valid(truncateJSON([]byte(`{"ok":true}`), 80)) {
		t.Fatal("small payload")
	}
}

func TestRedactJSONDoesNotMutateInput(t *testing.T) {
	in := []byte(`{"password":"x","ok":true}`)
	orig := string(in)
	_ = RedactJSON(in)
	if string(in) != orig {
		t.Fatal("mutated input")
	}
}

func TestRedactJSONInvalid(t *testing.T) {
	got := RedactJSON([]byte("not-json"))
	if !strings.Contains(string(got), "invalid_json") {
		t.Fatalf("%s", got)
	}
}

func TestWriteNilSafe(t *testing.T) {
	var l *Logger
	l.Write(nil, Event{Category: CategoryHTTP, Action: "x"})
	(&Logger{}).Write(nil, Event{Category: CategoryHTTP, Action: "x"})
}

func TestNoteActorFillsAttachedBag(t *testing.T) {
	ctx := Attach(context.Background())
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	NoteActor(ctx, "admin", &id, nil)
	f := From(ctx)
	if f == nil || f.ActorType != "admin" || f.ActorID == nil || *f.ActorID != id {
		t.Fatalf("%+v", f)
	}
	NoteActor(context.Background(), "admin", &id, nil) // no bag: no panic
}

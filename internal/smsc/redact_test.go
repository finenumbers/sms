package smsc

import "testing"

func TestRedactSecrets(t *testing.T) {
	got, _ := RedactSecrets(map[string]any{
		"psw":    "secret-pass",
		"login":  "secret-login",
		"phones": "7999",
		"nested": map[string]any{"apikey": "k", "id": 1},
	}).(map[string]any)
	if got["psw"] != "[REDACTED]" || got["login"] != "[REDACTED]" {
		t.Fatalf("%#v", got)
	}
	if got["phones"] != "7999" {
		t.Fatal("phones must stay")
	}
	nested := got["nested"].(map[string]any)
	if nested["apikey"] != "[REDACTED]" || nested["id"] != 1 {
		t.Fatalf("nested %#v", nested)
	}
}

func TestToPhoneDigits(t *testing.T) {
	if ToPhoneDigits("+79991234567") != "79991234567" {
		t.Fatal(ToPhoneDigits("+79991234567"))
	}
}

func TestCanonicalPhoneDigits(t *testing.T) {
	if CanonicalPhoneDigits("+89139447008") != "79139447008" {
		t.Fatal(CanonicalPhoneDigits("+89139447008"))
	}
	if CanonicalPhoneDigits("9139447008") != "79139447008" {
		t.Fatal(CanonicalPhoneDigits("9139447008"))
	}
	if CanonicalPhoneE164("89607977373") != "+79607977373" {
		t.Fatal(CanonicalPhoneE164("89607977373"))
	}
	if CallbackPhoneRaw(map[string]any{"phones": "79607977373"}) != "79607977373" {
		t.Fatal("phones fallback")
	}
}

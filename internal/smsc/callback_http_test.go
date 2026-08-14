package smsc

import (
	"net/http"
	"net/url"
	"testing"
)

func TestMergeCallbackPayloadBodyWins(t *testing.T) {
	q := url.Values{"id": {"q-id"}, "phone": {"79990000000"}, "status": {"1"}}
	body := []byte("id=b-id&status=2")
	got := MergeCallbackPayload(q, body, "application/x-www-form-urlencoded")
	if got["id"] != "b-id" || got["status"] != "2" || got["phone"] != "79990000000" {
		t.Fatalf("%#v", got)
	}
}

func TestMergeCallbackPayloadJSON(t *testing.T) {
	got := MergeCallbackPayload(url.Values{"id": {"q"}}, []byte(`{"id":"j","phone":"7999"}`), "application/json")
	if got["id"] != "j" || got["phone"] != "7999" {
		t.Fatalf("%#v", got)
	}
}

func TestSignaturesFromRequestHeaders(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "/internal/smsc/callback", nil)
	req.Header.Set("X-SMSC-MD5", "abc")
	req.Header.Set("X-SMSC-SHA1", "def")
	sig := SignaturesFromRequest(req, map[string]any{"md5": "ignored"})
	if sig.MD5 != "abc" || sig.SHA1 != "def" {
		t.Fatalf("%#v", sig)
	}
}

package smschttp

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/smsc"
)

func signedPayload(secret string) (map[string]any, string) {
	payload := map[string]any{
		"id":     "1001",
		"phone":  "79991234567",
		"status": "1",
		"err":    "0",
	}
	base := "1001:79991234567:1:" + secret
	sum := md5.Sum([]byte(base))
	sig := hex.EncodeToString(sum[:])
	payload["md5"] = sig
	return payload, sig
}

func TestCallbackRequiresProvider(t *testing.T) {
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/internal/smsc/callback", nil)
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestCallbackRejectsBadSignature(t *testing.T) {
	p := smsc.New(smsc.Options{
		Config:      mustSMSC(t, "secret"),
		Persistence: smsc.NewMemory(),
	})
	h := &Handlers{Provider: p}
	req := httptest.NewRequest(http.MethodGet, "/internal/smsc/callback?id=1&phone=7999&status=1&md5=deadbeef", nil)
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCallbackAppliesWhenOnlyPhonePresent(t *testing.T) {
	secret := "callback-secret"
	p := smsc.New(smsc.Options{
		Config:      mustSMSC(t, secret),
		Persistence: smsc.NewMemory(),
	})
	h := &Handlers{Provider: p, Lookup: &lookup.Worker{}}
	payload := map[string]any{
		"phone":  "79139447008",
		"status": "1",
		"err":    "0",
	}
	base := ":79139447008:1:" + secret
	sum := md5.Sum([]byte(base))
	payload["md5"] = hex.EncodeToString(sum[:])
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/internal/smsc/callback", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["applied"] != false {
		t.Fatalf("no store: applied=%v", out["applied"])
	}
}

func TestCallbackUnknownIDReturns200AppliedFalse(t *testing.T) {
	secret := "callback-secret"
	p := smsc.New(smsc.Options{
		Config:      mustSMSC(t, secret),
		Persistence: smsc.NewMemory(),
	})
	h := &Handlers{Provider: p}
	payload, _ := signedPayload(secret)
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/internal/smsc/callback", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["applied"] != false {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if out["message_id"] != "1001" {
		t.Fatalf("message_id=%v", out["message_id"])
	}
}

func TestCallbackRejectsWrongMethod(t *testing.T) {
	h := &Handlers{Provider: smsc.New(smsc.Options{Config: mustSMSC(t, "s")})}
	req := httptest.NewRequest(http.MethodPut, "/internal/smsc/callback", nil)
	rec := httptest.NewRecorder()
	h.Callback(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d", rec.Code)
	}
}

func mustSMSC(t *testing.T, secret string) smsc.Config {
	t.Helper()
	cfg, err := smsc.Resolve(smsc.Input{Login: "u", Password: "p", CallbackSecret: secret})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

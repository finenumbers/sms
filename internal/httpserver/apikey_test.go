package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/identity"
)

func TestBearerToken(t *testing.T) {
	if got := bearerToken("Bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken("bearer abc"); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken("Basic abc"); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := bearerToken(""); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireScope(t *testing.T) {
	h := RequireScope(apikeys.ScopeSend)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req = req.WithContext(authctx.WithPrincipal(req.Context(), identity.Principal{
		Scopes: []string{apikeys.ScopeRead},
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	req = req.WithContext(authctx.WithPrincipal(req.Context(), identity.Principal{
		Scopes: []string{apikeys.ScopeSend},
	}))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRequireAPIKeyMissingHeader(t *testing.T) {
	h := RequireAPIKey(nil, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next")
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/messages", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

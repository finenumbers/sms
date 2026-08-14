package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"finenumbers/sms/internal/config"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/identity"
)

type stubResolver struct {
	p identity.Principal
}

func (s stubResolver) Resolve(context.Context, uuid.UUID, sqlcdb.SessionAudience) (identity.Principal, error) {
	return s.p, nil
}

func TestRequireAudienceRefreshesCookie(t *testing.T) {
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	cfg := config.Config{
		SessionTTL:      12 * time.Hour,
		CookieSecure:    true,
		AdminCookieName: "__Host-fn_admin_sid",
	}
	h := RequireAudience(stubResolver{p: identity.Principal{SessionID: id}}, cfg, sqlcdb.SessionAudienceAdmin)(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
	)
	req := httptest.NewRequest(http.MethodGet, "/admin/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: cfg.AdminCookieName, Value: id.String()})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	c := cookies[0]
	if c.Name != cfg.AdminCookieName || c.Value != id.String() {
		t.Fatalf("cookie %s=%s", c.Name, c.Value)
	}
	if c.MaxAge != int(cfg.SessionTTL.Seconds()) {
		t.Fatalf("maxage=%d", c.MaxAge)
	}
}

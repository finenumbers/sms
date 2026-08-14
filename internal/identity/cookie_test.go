package identity

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"finenumbers/sms/internal/config"
)

func TestSetSessionCookieHostPrefix(t *testing.T) {
	cfg := config.Config{
		SessionTTL:      12 * time.Hour,
		CookieSecure:    true,
		AdminCookieName: "__Host-fn_admin_sid",
	}
	rec := httptest.NewRecorder()
	id := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	SetSessionCookie(rec, cfg, "admin", id)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	c := cookies[0]
	if c.Name != "__Host-fn_admin_sid" || c.Value != id.String() {
		t.Fatalf("name=%s value=%s", c.Name, c.Value)
	}
	if !c.HttpOnly || !c.Secure || c.Path != "/" || c.Domain != "" {
		t.Fatalf("attrs httponly=%v secure=%v path=%q domain=%q", c.HttpOnly, c.Secure, c.Path, c.Domain)
	}
}

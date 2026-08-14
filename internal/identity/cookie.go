package identity

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"finenumbers/sms/internal/config"
)

func SetSessionCookie(w http.ResponseWriter, cfg config.Config, audience string, sessionID uuid.UUID) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(cfg, audience),
		Value:    sessionID.String(),
		Path:     "/",
		MaxAge:   int(cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func ClearSessionCookie(w http.ResponseWriter, cfg config.Config, audience string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName(cfg, audience),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
	})
}

func ReadSessionID(r *http.Request, cfg config.Config, audience string) (uuid.UUID, error) {
	c, err := r.Cookie(cookieName(cfg, audience))
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(c.Value)
}

func cookieName(cfg config.Config, audience string) string {
	if audience == "client" {
		return cfg.ClientCookieName
	}
	return cfg.AdminCookieName
}

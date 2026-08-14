package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/config"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
)

type sessionResolver interface {
	Resolve(ctx context.Context, sessionID uuid.UUID, want sqlcdb.SessionAudience) (identity.Principal, error)
}

func RequireAudience(svc sessionResolver, cfg config.Config, want sqlcdb.SessionAudience) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id, err := identity.ReadSessionID(r, cfg, string(want))
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			p, err := svc.Resolve(r.Context(), id, want)
			if err != nil {
				status := http.StatusUnauthorized
				code := "unauthorized"
				msg := "authentication required"
				if errors.Is(err, identity.ErrAccountDisabled) || errors.Is(err, identity.ErrClientSuspended) {
					status = http.StatusForbidden
					code = "forbidden"
					msg = "account disabled"
				}
				httpx.WriteError(w, status, code, msg)
				return
			}
			identity.SetSessionCookie(w, cfg, string(want), p.SessionID)
			next.ServeHTTP(w, r.WithContext(authctx.WithPrincipal(r.Context(), p)))
		})
	}
}

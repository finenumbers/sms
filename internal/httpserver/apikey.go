package httpserver

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/ratelimit"
)

const (
	publicAPIRPS   = 10
	publicAPIBurst = 20
)

func RequireAPIKey(keys *apikeys.Service, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			if keys == nil {
				httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "api keys unavailable")
				return
			}
			p, err := keys.Resolve(r.Context(), raw, httpx.ClientIP(r))
			if err != nil {
				status := http.StatusUnauthorized
				code := "unauthorized"
				msg := "authentication required"
				switch {
				case errors.Is(err, apikeys.ErrRevoked):
					status = http.StatusForbidden
					code = "forbidden"
					msg = "api key revoked"
				case errors.Is(err, apikeys.ErrCIDR):
					status = http.StatusForbidden
					code = "forbidden"
					msg = "ip not allowed"
				case errors.Is(err, identity.ErrClientSuspended):
					status = http.StatusForbidden
					code = "client_suspended"
					msg = "client is suspended"
				case errors.Is(err, apikeys.ErrInvalidToken):
				default:
					if log != nil {
						log.Error("api key resolve", "err", err)
					}
					httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
					return
				}
				httpx.WriteError(w, status, code, msg)
				return
			}
			next.ServeHTTP(w, r.WithContext(authctx.WithPrincipal(r.Context(), p)))
		})
	}
}

func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.Principal(r.Context())
			if !ok || !p.HasScope(scope) {
				httpx.WriteError(w, http.StatusForbidden, "forbidden", "missing scope "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func PublicAPIRateLimit(limiter *ratelimit.Limiter, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := authctx.Principal(r.Context())
			if !ok || p.APIKeyID == nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
				return
			}
			if limiter == nil {
				httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
				return
			}
			ok, retry, err := limiter.AllowRate(r.Context(), "rl:http:apikey:"+p.APIKeyID.String(), publicAPIRPS, publicAPIBurst)
			if err != nil {
				if log != nil {
					log.Error("public api rate limit", "err", err)
				}
				httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
				return
			}
			if !ok {
				if retry > 0 {
					w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retry.Seconds())+1))
				}
				httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(h string) string {
	h = strings.TrimSpace(h)
	const p = "Bearer "
	if len(h) < len(p) || !strings.EqualFold(h[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

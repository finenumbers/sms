package admin

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"finenumbers/sms/internal/apikeys"
	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/inventory"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/ratelimit"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/smsc"
)

type Handlers struct {
	Log          *slog.Logger
	Cfg          config.Config
	Ident        *identity.Service
	Audit        *audit.Logger
	Limiter      *ratelimit.Limiter
	Settings     *settings.Service
	Runexis      *runexis.Client
	Inventory    *inventory.Service
	Store        *db.Store
	Keys         *apikeys.Service
	Billing      *billing.Service
	Lookup       *lookup.Service
	LookupWorker *lookup.Worker
	SMSC         *smsc.Provider
	SMSCCache    *smsc.BalanceCache
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	ip := httpx.ClientIP(r)
	ua := httpx.UserAgent(r)
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !h.allowLogin(w, r, "admin", email) {
		return
	}

	out, err := h.Ident.LoginAdmin(r.Context(), identity.LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		IP:        ip,
		UserAgent: ua,
	})
	if err != nil {
		h.Audit.Write(r.Context(), audit.Event{
			ActorType:    sqlcdb.ActorTypeAdmin,
			Action:       "auth.login.failure",
			ResourceType: "admin_user",
			IP:           ip,
			UserAgent:    ua,
			Metadata:     map[string]any{"email": email},
		})
		writeAuthErr(w, err)
		return
	}
	identity.SetSessionCookie(w, h.Cfg, "admin", out.Session.ID)
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      &out.User.ID,
		Action:       "auth.login.success",
		ResourceType: "admin_user",
		ResourceID:   &out.User.ID,
		IP:           ip,
		UserAgent:    ua,
		Metadata:     map[string]any{"email": out.User.Email},
	})
	httpx.WriteJSON(w, http.StatusOK, adminMe(out.User.ID.String(), out.User.Email, out.User.Name))
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	_ = h.Ident.Logout(r.Context(), p.SessionID)
	identity.ClearSessionCookie(w, h.Cfg, "admin")
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "auth.logout",
		ResourceType: "session",
		ResourceID:   &p.SessionID,
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	id := ""
	if p.AdminUserID != nil {
		id = p.AdminUserID.String()
	}
	httpx.WriteJSON(w, http.StatusOK, adminMe(id, p.Email, p.Name))
}

func (h *Handlers) allowLogin(w http.ResponseWriter, r *http.Request, audience, email string) bool {
	if h.Limiter == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return false
	}
	ip := "unknown"
	if v := httpx.ClientIP(r); v != nil {
		ip = v.String()
	}
	key := fmt.Sprintf("rl:login:%s:%s:%s", audience, ip, email)
	ok, retry, err := h.Limiter.Allow(r.Context(), key, 5, 5*time.Minute)
	if err != nil {
		h.Log.Error("login rate limit", "err", err)
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "rate limiter unavailable")
		return false
	}
	if !ok {
		if retry > 0 {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retry.Seconds())+1))
		}
		httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many attempts")
		return false
	}
	return true
}

func writeAuthErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, identity.ErrInvalidCredentials):
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid credentials")
	case errors.Is(err, identity.ErrAccountDisabled), errors.Is(err, identity.ErrClientSuspended):
		httpx.WriteError(w, http.StatusForbidden, "forbidden", "account disabled")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

func adminMe(id, email, name string) map[string]any {
	return map[string]any{
		"id":    id,
		"email": email,
		"name":  name,
		"role":  "admin",
	}
}

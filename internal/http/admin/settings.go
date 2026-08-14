package admin

import (
	"errors"
	"net/http"
	"time"

	"finenumbers/sms/internal/audit"
	"finenumbers/sms/internal/authctx"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/settings"
)

type settingsPatchRequest struct {
	RunexisEmail             *string              `json:"runexis_email"`
	RunexisPassword          *string              `json:"runexis_password"`
	CallbackBaseURL          *string              `json:"callback_base_url"`
	SMSDirections            *settings.Directions `json:"sms_directions"`
	ProviderRPS              *float64             `json:"provider_rps"`
	ClientRPSDefault         *float64             `json:"client_rps_default"`
	RetentionDays            *int32               `json:"retention_days"`
	AuditRetentionDays       *int32               `json:"audit_retention_days"`
	OpsRetentionDays         *int32               `json:"ops_retention_days"`
	BillingEnforced          *bool                `json:"billing_enforced"`
	LowBalanceThreshold      *string              `json:"low_balance_threshold"`
	RotateIngressToken       *bool                `json:"rotate_ingress_token"`
	LookupEnabled            *bool                `json:"lookup_enabled"`
	LookupCheckTimeoutSec    *int32               `json:"lookup_check_timeout_sec"`
	LookupPollIntervalSec    *int32               `json:"lookup_poll_interval_sec"`
	LookupMaxCSVRows         *int32               `json:"lookup_max_csv_rows"`
	LookupMaxCSVBytes        *int32               `json:"lookup_max_csv_bytes"`
	LookupMaxBatchPhones     *int32               `json:"lookup_max_batch_phones"`
	LookupWebhookMaxAttempts *int32               `json:"lookup_webhook_max_attempts"`
	LookupWebhookTimeoutMs   *int32               `json:"lookup_webhook_timeout_ms"`
	LookupRetentionDays      *int32               `json:"lookup_retention_days"`
	SMSCBaseURL              *string              `json:"smsc_base_url"`
	SMSCLogin                *string              `json:"smsc_login"`
	SMSCPassword             *string              `json:"smsc_password"`
	SMSCAPIKey               *string              `json:"smsc_apikey"`
	SMSCCallbackSecret       *string              `json:"smsc_callback_secret"`
	SMSCCurrency             *string              `json:"smsc_currency"`
}

func (h *Handlers) GetSettings(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "settings unavailable")
		return
	}
	view, err := h.Settings.Get(r.Context())
	if err != nil {
		h.Log.Error("get settings", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	applySettingsURLDefaults(&view, h.Cfg)
	httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *Handlers) PatchSettings(w http.ResponseWriter, r *http.Request) {
	if h.Settings == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "settings unavailable")
		return
	}
	p, ok := authctx.Principal(r.Context())
	if !ok || p.AdminUserID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	var req settingsPatchRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	view, credsChanged, err := h.Settings.Update(r.Context(), settings.Patch{
		RunexisEmail:             req.RunexisEmail,
		RunexisPassword:          req.RunexisPassword,
		CallbackBaseURL:          req.CallbackBaseURL,
		SMSDirections:            req.SMSDirections,
		ProviderRPS:              req.ProviderRPS,
		ClientRPSDefault:         req.ClientRPSDefault,
		RetentionDays:            req.RetentionDays,
		AuditRetentionDays:       req.AuditRetentionDays,
		OpsRetentionDays:         req.OpsRetentionDays,
		BillingEnforced:          req.BillingEnforced,
		LowBalanceThreshold:      req.LowBalanceThreshold,
		RotateIngressToken:       req.RotateIngressToken != nil && *req.RotateIngressToken,
		LookupEnabled:            req.LookupEnabled,
		LookupCheckTimeoutSec:    req.LookupCheckTimeoutSec,
		LookupPollIntervalSec:    req.LookupPollIntervalSec,
		LookupMaxCSVRows:         req.LookupMaxCSVRows,
		LookupMaxCSVBytes:        req.LookupMaxCSVBytes,
		LookupMaxBatchPhones:     req.LookupMaxBatchPhones,
		LookupWebhookMaxAttempts: req.LookupWebhookMaxAttempts,
		LookupWebhookTimeoutMs:   req.LookupWebhookTimeoutMs,
		LookupRetentionDays:      req.LookupRetentionDays,
		SMSCBaseURL:              req.SMSCBaseURL,
		SMSCLogin:                req.SMSCLogin,
		SMSCPassword:             req.SMSCPassword,
		SMSCAPIKey:               req.SMSCAPIKey,
		SMSCCallbackSecret:       req.SMSCCallbackSecret,
		SMSCCurrency:             req.SMSCCurrency,
		UpdatedBy:                *p.AdminUserID,
	})
	if err != nil {
		if errors.Is(err, settings.ErrValidation) {
			httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
			return
		}
		h.Log.Error("patch settings", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if credsChanged && h.Runexis != nil {
		_ = h.Runexis.Invalidate(r.Context())
	}
	meta := map[string]any{
		"password_updated":      req.RunexisPassword != nil,
		"email_updated":         req.RunexisEmail != nil,
		"ingress_token_rotated": req.RotateIngressToken != nil && *req.RotateIngressToken,
		"smsc_updated":          req.SMSCLogin != nil || req.SMSCPassword != nil || req.SMSCAPIKey != nil || req.SMSCCallbackSecret != nil,
		"lookup_enabled":        view.LookupEnabled,
	}
	if req.RunexisEmail != nil {
		meta["runexis_email"] = view.RunexisEmail
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "settings.update",
		ResourceType: "system_settings",
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     meta,
	})
	applySettingsURLDefaults(&view, h.Cfg)
	httpx.WriteJSON(w, http.StatusOK, view)
}

func (h *Handlers) TestRunexis(w http.ResponseWriter, r *http.Request) {
	if h.Runexis == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "runexis adapter unavailable")
		return
	}
	p, ok := authctx.Principal(r.Context())
	if !ok || p.AdminUserID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	acc, err := h.Runexis.TestAuth(r.Context())
	if err != nil {
		code := "runexis_error"
		status := http.StatusBadGateway
		msg := "runexis request failed"
		switch {
		case errors.Is(err, runexis.ErrNotConfigured), errors.Is(err, settings.ErrNotConfigured):
			status = http.StatusConflict
			code = "not_configured"
			msg = "runexis credentials are not configured"
		case errors.Is(err, settings.ErrDecrypt):
			status = http.StatusConflict
			code = "decrypt_failed"
			msg = "не удалось расшифровать секрет; введите его снова после смены ключа"
		default:
			var apiErr *runexis.APIError
			if errors.As(err, &apiErr) && apiErr.Message != "" {
				msg = apiErr.Message
			}
		}
		h.Audit.Write(r.Context(), audit.Event{
			ActorType:    sqlcdb.ActorTypeAdmin,
			ActorID:      p.AdminUserID,
			Action:       "settings.runexis.test.failure",
			ResourceType: "system_settings",
			IP:           httpx.ClientIP(r),
			UserAgent:    httpx.UserAgent(r),
		})
		httpx.WriteError(w, status, code, msg)
		return
	}
	out := map[string]any{
		"ok":           true,
		"email":        acc.Email,
		"name":         acc.Name,
		"statistic_ok": false,
	}
	now := time.Now().UTC()
	if _, err := h.Runexis.Statistic(r.Context(), runexis.StatisticQuery{
		From:  now.Add(-time.Hour),
		To:    now,
		Page:  1,
		Limit: 1,
	}); err != nil {
		msg := err.Error()
		var apiErr *runexis.APIError
		if errors.As(err, &apiErr) && apiErr.Message != "" {
			msg = apiErr.Message
		}
		out["statistic_error"] = msg
	} else {
		out["statistic_ok"] = true
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "settings.runexis.test.success",
		ResourceType: "system_settings",
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"runexis_email": acc.Email, "statistic_ok": out["statistic_ok"]},
	})
	httpx.WriteJSON(w, http.StatusOK, out)
}

type registerCallbacksRequest struct {
	IngressToken string `json:"ingress_token"`
}

func (h *Handlers) RegisterCallbacks(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.AdminUserID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if h.Settings == nil || h.Runexis == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "unavailable")
		return
	}
	var req registerCallbacksRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	view, err := h.Settings.Get(r.Context())
	if err != nil {
		h.Log.Error("get settings", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if view.CallbackBaseURL == "" {
		httpx.WriteError(w, http.StatusConflict, "not_configured", "callback_base_url is not set")
		return
	}
	if err := settings.ValidatePublicCallbackBase(view.CallbackBaseURL); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "callback_base_url must be a public https URL that Runexis can reach")
		return
	}
	stored, err := h.Settings.IngressHash(r.Context())
	if err != nil || stored == "" || !ingress.TokenMatch(stored, req.IngressToken) {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "ingress_token does not match")
		return
	}
	dlr, mo := settings.CallbackURLs(view.CallbackBaseURL, req.IngressToken)
	if err := h.Runexis.SetDLRURL(r.Context(), dlr); err != nil {
		writeRunexisErr(w, h, "register dlr-url", err)
		return
	}
	if err := h.Runexis.SetHookURL(r.Context(), mo); err != nil {
		writeRunexisErr(w, h, "register hook-url", err)
		return
	}
	h.Audit.Write(r.Context(), audit.Event{
		ActorType:    sqlcdb.ActorTypeAdmin,
		ActorID:      p.AdminUserID,
		Action:       "settings.callbacks.register",
		ResourceType: "system_settings",
		IP:           httpx.ClientIP(r),
		UserAgent:    httpx.UserAgent(r),
		Metadata:     map[string]any{"registered": true},
	})
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "dlr_url": dlr, "hook_url": mo})
}

func writeRunexisErr(w http.ResponseWriter, h *Handlers, op string, err error) {
	msg := "runexis request failed"
	var apiErr *runexis.APIError
	if errors.As(err, &apiErr) && apiErr.Message != "" {
		msg = apiErr.Message
	}
	h.Log.Error(op, "err", err)
	httpx.WriteError(w, http.StatusBadGateway, "runexis_error", msg)
}

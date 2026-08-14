package smschttp

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/ratelimit"
	"finenumbers/sms/internal/smsc"
)

const callbackRPM = 600

type Handlers struct {
	Log      *slog.Logger
	Provider *smsc.Provider
	Lookup   *lookup.Worker
	Limiter  *ratelimit.Limiter
	Ops      *ops.Logger
}

func (h *Handlers) Callback(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodPost:
	default:
		httpx.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if h.Provider == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_signature", "SMSC callback secret is not configured")
		return
	}
	if h.Limiter != nil {
		ip := "unknown"
		if v := httpx.ClientIP(r); v != nil {
			ip = v.String()
		}
		ok, retry, err := h.Limiter.Allow(r.Context(), "rl:smsc-cb:"+ip, callbackRPM, time.Minute)
		if err == nil && !ok {
			if retry > 0 {
				w.Header().Set("Retry-After", "1")
			}
			httpx.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
	}

	body, err := ingress.ReadBody(r)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_body", "cannot read body")
		return
	}
	if len(body) > ingress.MaxBody {
		httpx.WriteError(w, http.StatusRequestEntityTooLarge, "too_large", "body too large")
		return
	}

	payload := smsc.MergeCallbackPayload(r.URL.Query(), body, r.Header.Get("Content-Type"))
	result, err := h.Provider.HandleCallback(r.Context(), smsc.CallbackInput{
		RawPayload:    payload,
		Signatures:    smsc.SignaturesFromRequest(r, payload),
		CorrelationID: r.Header.Get("X-Request-Id"),
	})
	if err != nil {
		if pe := smsc.AsError(err); pe != nil && pe.Kind == smsc.KindSignature {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid_signature", pe.Message)
			return
		}
		if h.Log != nil {
			h.Log.Error("smsc callback persist", "err", err)
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}

	applied := false
	duplicate := false
	reason := ""
	phoneDigits := lookup.CallbackPhoneDigits(result.Normalized.PhoneE164)
	if h.Lookup != nil && (result.ProviderMessageID != "" || phoneDigits != "") {
		res, applyErr := h.Lookup.ApplyIncoming(r.Context(), lookup.IncomingCallback{
			ProviderMessageID: result.ProviderMessageID,
			PhoneDigits:       phoneDigits,
			Normalized:        result.Normalized,
			SkipEnrich:        true,
		})
		if applyErr != nil && h.Log != nil {
			h.Log.Error("smsc callback apply", "err", applyErr)
		} else {
			applied = res.Applied || res.Duplicate
			duplicate = res.Duplicate
			reason = res.Reason
			if lookup.ShouldConcludeCallback(res) {
				if callbackID, parseErr := uuid.Parse(result.ProviderCallbackID); parseErr == nil {
					if err := h.Lookup.ConcludeCallback(r.Context(), callbackID, res); err != nil && h.Log != nil {
						h.Log.Error("smsc callback conclude", "err", err)
					}
				}
			}
		}
	}

	if h.Ops != nil {
		h.Ops.Write(r.Context(), ops.Event{
			Category:   ops.CategoryIngress,
			Level:      ops.LevelInfo,
			Action:     "smsc.callback",
			HTTPMethod: r.Method,
			HTTPPath:   "/internal/smsc/callback",
			HTTPStatus: http.StatusOK,
			Summary:    "smsc callback",
			Detail: map[string]any{
				"applied":    applied,
				"duplicate":  duplicate,
				"reason":     reason,
				"deduped":    result.Deduplicated,
				"message_id": result.ProviderMessageID,
			},
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"applied":    applied,
		"duplicate":  duplicate,
		"deduped":    result.Deduplicated,
		"message_id": result.ProviderMessageID,
	})
}

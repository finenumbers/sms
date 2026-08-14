package lookup

import (
	"errors"
	"net/http"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/httpx"
)

func WriteError(w http.ResponseWriter, err error) {
	if httpx.WriteBillingError(w, err) {
		return
	}
	var le *Error
	if errors.As(err, &le) {
		switch {
		case errors.Is(le.Err, ErrValidation), le.Code == "validation":
			httpx.WriteError(w, http.StatusBadRequest, "validation", le.Message)
		case errors.Is(le.Err, ErrLookupDisabled), le.Code == "lookup_disabled":
			httpx.WriteError(w, http.StatusForbidden, "lookup_disabled", le.Message)
		case errors.Is(le.Err, ErrClientSuspended), le.Code == "client_suspended":
			httpx.WriteError(w, http.StatusForbidden, "client_suspended", le.Message)
		case errors.Is(le.Err, ErrNotFound), le.Code == "not_found":
			httpx.WriteError(w, http.StatusNotFound, "not_found", le.Message)
		case errors.Is(le.Err, ErrConflict), le.Code == "conflict":
			httpx.WriteError(w, http.StatusConflict, "conflict", le.Message)
		case errors.Is(le.Err, ErrTariffNotConfigured), le.Code == "tariff_not_configured":
			httpx.WriteError(w, http.StatusConflict, "tariff_not_configured", le.Message)
		case errors.Is(le.Err, billing.ErrInsufficientFunds), le.Code == "insufficient_funds":
			httpx.WriteError(w, http.StatusPaymentRequired, "insufficient_funds", le.Message)
		default:
			httpx.WriteError(w, http.StatusBadRequest, le.Code, le.Message)
		}
		return
	}
	switch {
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, ErrLookupDisabled):
		httpx.WriteError(w, http.StatusForbidden, "lookup_disabled", err.Error())
	case errors.Is(err, ErrClientSuspended):
		httpx.WriteError(w, http.StatusForbidden, "client_suspended", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrTariffNotConfigured):
		httpx.WriteError(w, http.StatusConflict, "tariff_not_configured", err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

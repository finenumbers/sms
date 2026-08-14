package httpx

import (
	"errors"
	"net/http"

	"finenumbers/sms/internal/billing"
)

// WriteBillingError maps prepaid wallet errors. Returns true if err was a billing error.
func WriteBillingError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, billing.ErrInsufficientFunds):
		WriteError(w, http.StatusPaymentRequired, "insufficient_funds", err.Error())
		return true
	case errors.Is(err, billing.ErrTariffNotConfigured):
		WriteError(w, http.StatusConflict, "tariff_not_configured", err.Error())
		return true
	case errors.Is(err, billing.ErrInvalidTariff):
		WriteError(w, http.StatusConflict, "invalid_tariff", err.Error())
		return true
	case errors.Is(err, billing.ErrNegativeBalance):
		WriteError(w, http.StatusConflict, "negative_balance_forbidden", err.Error())
		return true
	case errors.Is(err, billing.ErrWalletNotFound):
		WriteError(w, http.StatusNotFound, "wallet_not_found", err.Error())
		return true
	case errors.Is(err, billing.ErrValidation), errors.Is(err, billing.ErrInvalidAmount):
		WriteError(w, http.StatusBadRequest, "validation", err.Error())
		return true
	default:
		return false
	}
}

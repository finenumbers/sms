package billing

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

var (
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrTariffNotConfigured  = errors.New("tariff not configured")
	ErrInvalidTariff        = errors.New("invalid tariff")
	ErrPriceSnapshotMissing = errors.New("price snapshot missing")
	ErrWalletNotFound       = errors.New("wallet not found")
	ErrHoldNotFound         = errors.New("hold not found")
	ErrInvalidAmount        = errors.New("invalid amount")
	ErrNegativeBalance      = errors.New("negative balance forbidden")
	ErrValidation           = errors.New("validation")
)

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error { return e.Err }

func wrap(err error, code, msg string) error {
	return &Error{Code: code, Message: msg, Err: err}
}

func money(s string) (decimal.Decimal, error) {
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return decimal.Zero, fmt.Errorf("%w: %s", ErrInvalidAmount, s)
	}
	return d, nil
}

func mustPositive(d decimal.Decimal, field string) error {
	if d.LessThanOrEqual(decimal.Zero) {
		return fmt.Errorf("%w: %s must be > 0", ErrInvalidAmount, field)
	}
	return nil
}

func moneyString(d decimal.Decimal) string {
	return FormatMoney(d)
}

func FormatMoney(d decimal.Decimal) string {
	return d.StringFixed(6)
}

func FormatMoneyPtr(d *decimal.Decimal) any {
	if d == nil {
		return nil
	}
	return FormatMoney(*d)
}

package lookup

import (
	"errors"
	"fmt"
)

var (
	ErrValidation          = errors.New("validation")
	ErrLookupDisabled      = errors.New("lookup disabled")
	ErrClientSuspended     = errors.New("client suspended")
	ErrNotFound            = errors.New("not found")
	ErrConflict            = errors.New("conflict")
	ErrTariffNotConfigured = errors.New("tariff not configured")
)

type Error struct {
	Code    string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func wrap(err error, code, msg string) error {
	return &Error{Code: code, Message: msg, Err: err}
}

func AsError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

func fmtWrap(err error, code, format string, args ...any) error {
	return wrap(err, code, fmt.Sprintf(format, args...))
}

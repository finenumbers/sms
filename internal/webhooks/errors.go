package webhooks

import (
	"errors"
	"net/http"

	"finenumbers/sms/internal/httpx"
)

var (
	ErrValidation = errors.New("validation")
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("conflict")
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

func WriteError(w http.ResponseWriter, err error) {
	var e *Error
	if errors.As(err, &e) {
		switch {
		case errors.Is(e.Err, ErrValidation), e.Code == "validation":
			httpx.WriteError(w, http.StatusBadRequest, "validation", e.Message)
		case errors.Is(e.Err, ErrNotFound), e.Code == "not_found":
			httpx.WriteError(w, http.StatusNotFound, "not_found", e.Message)
		case errors.Is(e.Err, ErrConflict), e.Code == "conflict":
			httpx.WriteError(w, http.StatusConflict, "conflict", e.Message)
		default:
			httpx.WriteError(w, http.StatusBadRequest, e.Code, e.Message)
		}
		return
	}
	switch {
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, http.StatusBadRequest, "validation", err.Error())
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

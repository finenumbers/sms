package smsc

import (
	"errors"
	"fmt"
	"strconv"
)

type Kind string

const (
	KindAuth              Kind = "auth"
	KindValidation        Kind = "validation"
	KindInsufficientFunds Kind = "insufficient_funds"
	KindRateLimit         Kind = "rate_limit"
	KindTimeout           Kind = "timeout"
	KindNetwork           Kind = "network"
	KindProvider          Kind = "provider"
	KindSignature         Kind = "signature"
	KindConflict          Kind = "conflict"
	KindUnknown           Kind = "unknown"
)

type Error struct {
	ProviderCode         string
	Kind                 Kind
	Message              string
	ProviderErrorCode    any
	ProviderErrorMessage string
	HTTPStatus           int
	Retryable            bool
	CorrelationID        string
	RawResponse          any
	Err                  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

func (e *Error) Unwrap() error { return e.Err }

func AsError(err error) *Error {
	var pe *Error
	if errors.As(err, &pe) {
		return pe
	}
	return nil
}

type mappedCode struct {
	Kind      Kind
	Retryable bool
}

// MapErrorCode classifies SMSC send/status error_code values.
// 1 params, 2 auth, 3 funds, 4 IP, 5 date, 6 forbidden, 7 phone,
// 8 cannot deliver, 9 duplicate flood.
func MapErrorCode(code any) mappedCode {
	n, ok := asInt(code)
	if !ok {
		return mappedCode{Kind: KindProvider, Retryable: false}
	}
	switch n {
	case 2, 4:
		return mappedCode{Kind: KindAuth, Retryable: false}
	case 3:
		return mappedCode{Kind: KindInsufficientFunds, Retryable: false}
	case 1, 5, 7:
		return mappedCode{Kind: KindValidation, Retryable: false}
	case 6, 8:
		return mappedCode{Kind: KindProvider, Retryable: false}
	case 9:
		return mappedCode{Kind: KindRateLimit, Retryable: true}
	default:
		return mappedCode{Kind: KindProvider, Retryable: false}
	}
}

func isErrorBody(body any) bool {
	obj, ok := asObject(body)
	if !ok {
		return false
	}
	if _, has := obj["error_code"]; has {
		return true
	}
	if _, hasErr := obj["error"]; !hasErr {
		return false
	}
	_, hasStatus := obj["status"]
	_, hasID := obj["id"]
	return !(hasStatus && hasID)
}

func errorFromBody(body any, correlationID string, httpStatus int) *Error {
	obj, _ := asObject(body)
	code := obj["error_code"]
	msg, _ := obj["error"].(string)
	mapped := MapErrorCode(code)
	text := fmt.Sprintf("Provider error_code=%v", orUnknown(code))
	if msg != "" {
		text = fmt.Sprintf("Provider error %v: %s", orUnknown(code), msg)
	}
	return &Error{
		ProviderCode:         ProviderCode,
		Kind:                 mapped.Kind,
		Message:              text,
		ProviderErrorCode:    normalizeErrorCode(code),
		ProviderErrorMessage: msg,
		HTTPStatus:           httpStatus,
		Retryable:            mapped.Retryable,
		CorrelationID:        correlationID,
		RawResponse:          body,
	}
}

func assertNoError(body any, correlationID string, httpStatus int) error {
	obj, ok := asObject(body)
	if !ok {
		return nil
	}
	if hasNonEmpty(obj, "error_code") {
		return errorFromBody(obj, correlationID, httpStatus)
	}
	if msg, _ := obj["error"].(string); msg != "" {
		if _, hasID := obj["id"]; !hasID {
			if _, hasStatus := obj["status"]; !hasStatus {
				return errorFromBody(obj, correlationID, httpStatus)
			}
		}
	}
	return nil
}

func hasNonEmpty(obj map[string]any, key string) bool {
	v, ok := obj[key]
	if !ok || v == nil {
		return false
	}
	if s, isStr := v.(string); isStr && s == "" {
		return false
	}
	return true
}

func orUnknown(v any) any {
	if v == nil {
		return "unknown"
	}
	return v
}

func normalizeErrorCode(v any) any {
	if v == nil {
		return nil
	}
	if n, ok := asInt(v); ok {
		return n
	}
	return v
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n == float64(int(n)) {
			return int(n), true
		}
	case string:
		i, err := strconv.Atoi(n)
		if err == nil {
			return i, true
		}
	}
	return 0, false
}

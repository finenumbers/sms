package smsc

import "testing"

func TestMapErrorCode(t *testing.T) {
	if got := MapErrorCode(2); got.Kind != KindAuth || got.Retryable {
		t.Fatalf("auth: %#v", got)
	}
	if got := MapErrorCode(3); got.Kind != KindInsufficientFunds || got.Retryable {
		t.Fatalf("funds: %#v", got)
	}
	if got := MapErrorCode(7); got.Kind != KindValidation || got.Retryable {
		t.Fatalf("validation: %#v", got)
	}
	if got := MapErrorCode(9); got.Kind != KindRateLimit || !got.Retryable {
		t.Fatalf("rate limit: %#v", got)
	}
}

func TestErrorFromBodyPreservesCode(t *testing.T) {
	err := errorFromBody(map[string]any{
		"error":      "invalid login or password",
		"error_code": 2,
	}, "", 0)
	if err.ProviderErrorCode != 2 {
		t.Fatalf("code %#v", err.ProviderErrorCode)
	}
	if err.ProviderErrorMessage != "invalid login or password" {
		t.Fatalf("msg %q", err.ProviderErrorMessage)
	}
	if err.Kind != KindAuth || err.Retryable {
		t.Fatalf("kind %s retryable %v", err.Kind, err.Retryable)
	}
}

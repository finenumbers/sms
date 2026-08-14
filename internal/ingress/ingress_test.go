package ingress

import (
	"net/http"
	"strings"
	"testing"
)

func TestIdempotencyKeyStable(t *testing.T) {
	a := IdempotencyKey("POST", "/internal/runexis/dlr/tok", "a=1", []byte(`{"x":1}`))
	b := IdempotencyKey("POST", "/internal/runexis/dlr/tok", "a=1", []byte(`{"x":1}`))
	if a != b || a == "" {
		t.Fatalf("%s vs %s", a, b)
	}
	c := IdempotencyKey("GET", "/internal/runexis/dlr/tok", "a=1", []byte(`{"x":1}`))
	if a == c {
		t.Fatal("method should change hash")
	}
}

func TestTokenMatch(t *testing.T) {
	h := HashToken("secret-token")
	if !TokenMatch(h, "secret-token") {
		t.Fatal("match")
	}
	if TokenMatch(h, "other") || TokenMatch("", "secret-token") || TokenMatch(h, "") {
		t.Fatal("mismatch")
	}
}

func TestRedactPath(t *testing.T) {
	got := RedactPath("/internal/runexis/dlr/supersecrettoken")
	if got != "/internal/runexis/dlr/*" {
		t.Fatalf("got %s", got)
	}
	if strings.Contains(got, "secret") {
		t.Fatal("token leaked")
	}
	if RedactPath("/admin/v1/settings") != "/admin/v1/settings" {
		t.Fatal("other paths")
	}
}

func TestSanitizeHeadersDropsSecrets(t *testing.T) {
	h := http.Header{
		"Authorization":    []string{"Bearer x"},
		"Cookie":           []string{"sid=1"},
		"X-Requested-With": []string{"XMLHttpRequest"},
	}
	b := SanitizeHeaders(h)
	s := string(b)
	if strings.Contains(s, "Bearer") || strings.Contains(strings.ToLower(s), "cookie") {
		t.Fatalf("leaked: %s", s)
	}
	if !strings.Contains(s, "XMLHttpRequest") {
		t.Fatalf("kept header missing: %s", s)
	}
}

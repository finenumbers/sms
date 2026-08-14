package idempotency

import "testing"

func TestHashRequestStable(t *testing.T) {
	a := HashRequest("POST", "/v1/messages", []byte(`{"from":"1"}`))
	b := HashRequest("POST", "/v1/messages", []byte(`{"from":"1"}`))
	c := HashRequest("POST", "/v1/messages", []byte(`{"from":"2"}`))
	if a != b {
		t.Fatal("expected stable hash")
	}
	if a == c {
		t.Fatal("different body should differ")
	}
	if len(a) != 64 {
		t.Fatalf("len=%d", len(a))
	}
}

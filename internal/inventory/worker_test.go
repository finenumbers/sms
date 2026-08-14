package inventory

import (
	"testing"
	"time"
)

func TestBackoff(t *testing.T) {
	if backoff(1) != 5*time.Second {
		t.Fatalf("attempt 1: %s", backoff(1))
	}
	if backoff(20) != 5*time.Minute {
		t.Fatalf("cap: %s", backoff(20))
	}
}

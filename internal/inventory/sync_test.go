package inventory

import (
	"context"
	"errors"
	"testing"

	"finenumbers/sms/internal/runexis"
)

func TestSyncFromProviderNotConfigured(t *testing.T) {
	s := New(nil, nil, nil)
	_, err := s.SyncFromProvider(context.Background())
	if !errors.Is(err, runexis.ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

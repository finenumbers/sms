package smsc

import (
	"context"
	"testing"
	"time"
)

func TestPersistContextSurvivesParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	if err := parent.Err(); err == nil {
		t.Fatal("parent must be cancelled")
	}
	ctx, stop := persistContext(parent)
	defer stop()
	if err := ctx.Err(); err != nil {
		t.Fatalf("write after SMSC HTTP must not inherit Tick cancel: %v", err)
	}
	dl, ok := ctx.Deadline()
	if !ok || time.Until(dl) > persistTimeout+50*time.Millisecond {
		t.Fatal("persist ctx must cap the write")
	}
	mem := NewMemory()
	if _, err := mem.SaveRequest(ctx, RequestRecord{
		ProviderCode: ProviderCode,
		Kind:         KindBalance,
		Status:       RequestPending,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPersistContextNilParent(t *testing.T) {
	ctx, stop := persistContext(nil)
	defer stop()
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
}

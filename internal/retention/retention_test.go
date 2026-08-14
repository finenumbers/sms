package retention

import (
	"context"
	"testing"
	"time"
)

func TestTickSkipsWithinHour(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	w := &Worker{
		now:  func() time.Time { return now },
		last: now.Add(-30 * time.Minute),
	}
	if err := w.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w.last != now.Add(-30*time.Minute) {
		t.Fatal("must not run")
	}
}

package app

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitWithTimeoutReturnsWhenWaitFinishes(t *testing.T) {
	waitWithTimeout(nil, "test", time.Second, func() {})
}

func TestWaitWithTimeoutDoesNotBlockPastDeadline(t *testing.T) {
	var finished atomic.Bool
	started := time.Now()
	waitWithTimeout(nil, "test", 30*time.Millisecond, func() {
		time.Sleep(time.Second)
		finished.Store(true)
	})
	if time.Since(started) > 300*time.Millisecond {
		t.Fatal("drain must return after timeout, not after the stuck loop")
	}
	if finished.Load() {
		t.Fatal("stuck wait must not have finished")
	}
}

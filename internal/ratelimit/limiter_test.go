package ratelimit

import (
	"context"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestAllowWindow(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	l := New(rdb)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ok, _, err := l.Allow(ctx, "rl:test", 5, time.Minute)
		if err != nil || !ok {
			t.Fatalf("attempt %d: ok=%v err=%v", i+1, ok, err)
		}
	}
	ok, retry, err := l.Allow(ctx, "rl:test", 5, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected rate limit")
	}
	if retry <= 0 {
		t.Fatalf("retryAfter=%s", retry)
	}
}

func TestAllowRate(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	l := New(rdb)
	ctx := context.Background()
	ok, _, err := l.AllowRate(ctx, "rl:tb", 10, 2)
	if err != nil || !ok {
		t.Fatalf("first: ok=%v err=%v", ok, err)
	}
	ok, _, err = l.AllowRate(ctx, "rl:tb", 10, 2)
	if err != nil || !ok {
		t.Fatalf("second burst: ok=%v err=%v", ok, err)
	}
	ok, retry, err := l.AllowRate(ctx, "rl:tb", 10, 2)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected empty bucket")
	}
	if retry <= 0 {
		t.Fatalf("retry=%s", retry)
	}
	if err := l.Drain(ctx, "rl:tb"); err != nil {
		t.Fatal(err)
	}
}

func TestTryLock(t *testing.T) {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	l := New(rdb)
	ctx := context.Background()
	ok, err := l.TryLock(ctx, "campaign:start:x", time.Second)
	if err != nil || !ok {
		t.Fatalf("first lock: ok=%v err=%v", ok, err)
	}
	ok, err = l.TryLock(ctx, "campaign:start:x", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected lock held")
	}
	if err := l.Unlock(ctx, "campaign:start:x"); err != nil {
		t.Fatal(err)
	}
	ok, err = l.TryLock(ctx, "campaign:start:x", time.Second)
	if err != nil || !ok {
		t.Fatalf("after unlock: ok=%v err=%v", ok, err)
	}
}

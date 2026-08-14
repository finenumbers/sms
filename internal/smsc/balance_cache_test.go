package smsc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestBalanceCacheRoundTrip(t *testing.T) {
	mr := miniredis.RunT(t)
	c := NewBalanceCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()
	if err := c.Write(ctx, Balance{Balance: "12.50", Currency: "RUB", RawResponse: map[string]any{"password": "secret"}}); err != nil {
		t.Fatal(err)
	}
	raw, err := mr.Get(BalanceCacheKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "secret") || strings.Contains(raw, "password") {
		t.Fatalf("secret leaked into redis: %s", raw)
	}
	var row cachedBalance
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		t.Fatal(err)
	}
	if row.Balance != "12.50" || row.Currency != "RUB" {
		t.Fatalf("%+v", row)
	}
	n, ok, err := c.Read(ctx)
	if err != nil || !ok || n != 12.5 {
		t.Fatalf("read %v ok=%v err=%v", n, ok, err)
	}
}

func TestBalanceCacheMissAndEmpty(t *testing.T) {
	mr := miniredis.RunT(t)
	c := NewBalanceCache(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()
	if _, ok, err := c.Read(ctx); err != nil || ok {
		t.Fatal("empty redis must be a miss, not zero")
	}
	if err := c.Write(ctx, Balance{Balance: "  ", Currency: "RUB"}); err != nil {
		t.Fatal(err)
	}
	if mr.Exists(BalanceCacheKey) {
		t.Fatal("blank balance must not write a zero cache")
	}
}

func TestBalanceCacheKey(t *testing.T) {
	if BalanceCacheKey != "sms:provider:smsc:balance" {
		t.Fatalf("key %s", BalanceCacheKey)
	}
}

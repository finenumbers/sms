package ratelimit

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Limiter struct {
	rdb redis.Cmdable
}

func New(rdb redis.Cmdable) *Limiter {
	return &Limiter{rdb: rdb}
}

// Allow increments a window counter. limit is max successful increments in window.
func (l *Limiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, retryAfter time.Duration, err error) {
	if l == nil || l.rdb == nil {
		return false, window, fmt.Errorf("rate limiter unavailable")
	}
	pipe := l.rdb.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, err
	}
	n := incr.Val()
	if n > int64(limit) {
		ttl, err := l.rdb.TTL(ctx, key).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}

var tokenBucketLua = redis.NewScript(`
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local cost = tonumber(ARGV[4])
if rate == nil or rate <= 0 then
  return {1, 0}
end
if burst == nil or burst < 1 then
  burst = 1
end
local data = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(data[1])
local ts = tonumber(data[2])
if tokens == nil then
  tokens = burst
  ts = now
end
local elapsed = now - ts
if elapsed < 0 then
  elapsed = 0
end
tokens = math.min(burst, tokens + elapsed * rate)
if tokens < cost then
  redis.call('HSET', key, 'tokens', tokens, 'ts', now)
  redis.call('EXPIRE', key, 120)
  local waitms = math.ceil((cost - tokens) / rate * 1000)
  return {0, waitms}
end
tokens = tokens - cost
redis.call('HSET', key, 'tokens', tokens, 'ts', now)
redis.call('EXPIRE', key, 120)
return {1, 0}
`)

// AllowRate is a Redis token bucket. rate is tokens/sec; burst is max tokens.
func (l *Limiter) AllowRate(ctx context.Context, key string, rate, burst float64) (ok bool, retryAfter time.Duration, err error) {
	if l == nil || l.rdb == nil {
		return false, time.Second, fmt.Errorf("rate limiter unavailable")
	}
	now := float64(time.Now().UnixMilli()) / 1000.0
	res, err := tokenBucketLua.Run(ctx, l.rdb, []string{key}, rate, burst, now, 1).Slice()
	if err != nil {
		return false, 0, err
	}
	if len(res) < 2 {
		return false, 0, fmt.Errorf("token bucket: bad redis reply")
	}
	allowed, ok1 := int64Val(res[0])
	waitms, ok2 := int64Val(res[1])
	if !ok1 || !ok2 {
		return false, 0, fmt.Errorf("token bucket: bad redis reply types")
	}
	if allowed == 1 {
		return true, 0, nil
	}
	d := time.Duration(waitms) * time.Millisecond
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return false, d, nil
}

func (l *Limiter) TryLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	if l == nil || l.rdb == nil {
		return false, fmt.Errorf("rate limiter unavailable")
	}
	if ttl <= 0 {
		ttl = 15 * time.Second
	}
	return l.rdb.SetNX(ctx, key, "1", ttl).Result()
}

func (l *Limiter) Unlock(ctx context.Context, key string) error {
	if l == nil || l.rdb == nil {
		return fmt.Errorf("rate limiter unavailable")
	}
	return l.rdb.Del(ctx, key).Err()
}

func int64Val(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case uint64:
		return int64(t), true
	case float64:
		return int64(t), true
	case string:
		n, err := strconv.ParseInt(t, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

func (l *Limiter) Drain(ctx context.Context, key string) error {
	if l == nil || l.rdb == nil {
		return fmt.Errorf("rate limiter unavailable")
	}
	now := float64(time.Now().UnixMilli()) / 1000.0
	pipe := l.rdb.TxPipeline()
	pipe.HSet(ctx, key, "tokens", 0, "ts", now)
	pipe.Expire(ctx, key, 2*time.Minute)
	_, err := pipe.Exec(ctx)
	return err
}

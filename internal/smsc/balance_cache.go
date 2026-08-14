package smsc

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// BalanceCacheKey is the Redis key for the SMSC cabinet balance.
// Old HLR used hlr:provider:smsc:balance — do not reuse that name.
const BalanceCacheKey = "sms:provider:smsc:balance"

const BalanceCacheTTL = 3 * time.Minute

type cachedBalance struct {
	Balance   string `json:"balance"`
	Currency  string `json:"currency"`
	UpdatedAt string `json:"updated_at"`
}

type BalanceCache struct {
	rdb redis.Cmdable
}

func NewBalanceCache(rdb redis.Cmdable) *BalanceCache {
	if rdb == nil {
		return nil
	}
	return &BalanceCache{rdb: rdb}
}

func (c *BalanceCache) Write(ctx context.Context, bal Balance) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	amount := strings.TrimSpace(bal.Balance)
	if amount == "" {
		return nil
	}
	payload, err := json.Marshal(cachedBalance{
		Balance:   amount,
		Currency:  bal.Currency,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, BalanceCacheKey, payload, BalanceCacheTTL).Err()
}

func (c *BalanceCache) Read(ctx context.Context) (float64, bool, error) {
	if c == nil || c.rdb == nil {
		return 0, false, nil
	}
	raw, err := c.rdb.Get(ctx, BalanceCacheKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return 0, false, nil
		}
		return 0, false, err
	}
	var row cachedBalance
	if err := json.Unmarshal(raw, &row); err != nil {
		return 0, false, err
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(row.Balance), 64)
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}

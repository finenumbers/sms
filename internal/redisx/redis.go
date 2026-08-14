package redisx

import (
	"context"
	"fmt"
	"net/http"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func Connect(ctx context.Context, redisURL string) (*Client, error) {
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opt)
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return &Client{rdb: rdb}, nil
}

func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Cmdable() redis.Cmdable {
	if c == nil {
		return nil
	}
	return c.rdb
}

func (c *Client) Ready(_ *http.Request) error {
	if c == nil {
		return fmt.Errorf("redis")
	}
	if err := c.rdb.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("redis")
	}
	return nil
}

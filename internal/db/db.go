package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

type Store struct {
	Pool    *pgxpool.Pool
	Queries *sqlcdb.Queries
}

func Connect(ctx context.Context, databaseURL string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &Store{Pool: pool, Queries: sqlcdb.New(pool)}, nil
}

func (s *Store) Close() {
	if s != nil && s.Pool != nil {
		s.Pool.Close()
	}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.Pool.Ping(ctx)
}

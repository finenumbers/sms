package seed

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/password"
)

func Admin(ctx context.Context, log *slog.Logger, store *db.Store, cfg config.Config) error {
	email := strings.TrimSpace(cfg.SeedAdminEmail)
	if email == "" || cfg.SeedAdminPassword == "" {
		log.Info("admin seed skipped (SEED_ADMIN_EMAIL / SEED_ADMIN_PASSWORD not set)")
		return nil
	}
	n, err := store.Queries.CountAdminUsers(ctx)
	if err != nil {
		return fmt.Errorf("count admin users: %w", err)
	}
	if n > 0 {
		log.Info("admin seed skipped (users already exist)")
		return nil
	}
	hash, err := password.Hash(cfg.SeedAdminPassword)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	u, err := store.Queries.InsertAdminUser(ctx, sqlcdb.InsertAdminUserParams{
		Email:        email,
		PasswordHash: hash,
		Name:         cfg.SeedAdminName,
	})
	if err != nil {
		return fmt.Errorf("insert admin: %w", err)
	}
	log.Info("seeded admin user", "id", u.ID, "email", u.Email)
	return nil
}

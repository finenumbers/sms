package migrate

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"finenumbers/sms/db"
)

func Up(databaseURL string) error {
	src, err := iofs.New(db.Migrations, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	dsn, err := pgxMigrateURL(databaseURL)
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

func pgxMigrateURL(databaseURL string) (string, error) {
	// golang-migrate pgx/v5 driver expects scheme pgx5://
	switch {
	case len(databaseURL) >= 11 && databaseURL[:11] == "postgres://":
		return "pgx5://" + databaseURL[11:], nil
	case len(databaseURL) >= 13 && databaseURL[:13] == "postgresql://":
		return "pgx5://" + databaseURL[13:], nil
	case len(databaseURL) >= 7 && databaseURL[:7] == "pgx5://":
		return databaseURL, nil
	default:
		return "", fmt.Errorf("unsupported DATABASE_URL scheme")
	}
}

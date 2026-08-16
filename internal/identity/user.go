package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/password"
)

const (
	MaxClientUsers   = 20
	maxUserNameRunes = 120
)

type CreateClientUserInput struct {
	ClientID uuid.UUID
	Email    string
	Name     string
	Password string
}

func CanDisableActiveOwner(activeOwners int64) bool {
	return activeOwners > 1
}

func validateUserName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: name required", ErrValidation)
	}
	if utf8.RuneCountInString(name) > maxUserNameRunes {
		return "", fmt.Errorf("%w: name too long", ErrValidation)
	}
	return name, nil
}

func (s *Service) CreateClientUser(ctx context.Context, in CreateClientUserInput) (sqlcdb.ClientUser, error) {
	email := normalizeEmail(in.Email)
	if err := validateEmail(email); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	name, err := validateUserName(in.Name)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if len(in.Password) < 10 {
		return sqlcdb.ClientUser{}, fmt.Errorf("%w: password must be at least 10 characters", ErrValidation)
	}
	hash, err := password.Hash(in.Password)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}

	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	cl, err := q.GetClientByIDForUpdate(ctx, in.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.ClientUser{}, ErrNotFound
		}
		return sqlcdb.ClientUser{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return sqlcdb.ClientUser{}, ErrNotFound
	}
	n, err := q.CountClientUsersByClientID(ctx, in.ClientID)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if n >= MaxClientUsers {
		return sqlcdb.ClientUser{}, fmt.Errorf("%w: user limit reached", ErrConflict)
	}
	user, err := q.InsertClientUser(ctx, sqlcdb.InsertClientUserParams{
		ClientID:     in.ClientID,
		Email:        email,
		PasswordHash: hash,
		Name:         name,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return sqlcdb.ClientUser{}, ErrEmailTaken
		}
		return sqlcdb.ClientUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	return user, nil
}

func (s *Service) ResetClientUserPassword(ctx context.Context, clientID, userID uuid.UUID, newPassword string) error {
	if len(newPassword) < 10 {
		return fmt.Errorf("%w: password must be at least 10 characters", ErrValidation)
	}
	hash, err := password.Hash(newPassword)
	if err != nil {
		return err
	}
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)
	if _, err := loadClientUserForClient(ctx, q, clientID, userID); err != nil {
		return err
	}
	if err := q.UpdateClientUserPassword(ctx, sqlcdb.UpdateClientUserPasswordParams{
		PasswordHash: hash,
		ID:           userID,
	}); err != nil {
		return err
	}
	if err := q.RevokeSessionsForClientUser(ctx, &userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) UpdateClientUserName(ctx context.Context, clientID, userID uuid.UUID, name string) (sqlcdb.ClientUser, error) {
	name, err := validateUserName(name)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)
	if _, err := loadClientUserForClient(ctx, q, clientID, userID); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	out, err := q.UpdateClientUserName(ctx, sqlcdb.UpdateClientUserNameParams{Name: name, ID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.ClientUser{}, ErrNotFound
		}
		return sqlcdb.ClientUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	return out, nil
}

func (s *Service) DisableClientUser(ctx context.Context, clientID, userID uuid.UUID) (sqlcdb.ClientUser, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)
	user, err := loadClientUserForClient(ctx, q, clientID, userID)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if user.Status == sqlcdb.UserStatusDisabled {
		if err := tx.Commit(ctx); err != nil {
			return sqlcdb.ClientUser{}, err
		}
		return user, nil
	}
	active, err := q.CountActiveOwnersByClientID(ctx, clientID)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if !CanDisableActiveOwner(active) {
		return sqlcdb.ClientUser{}, fmt.Errorf("%w: last active owner", ErrConflict)
	}
	out, err := q.UpdateClientUserStatus(ctx, sqlcdb.UpdateClientUserStatusParams{
		Status: sqlcdb.UserStatusDisabled,
		ID:     userID,
	})
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if err := q.RevokeSessionsForClientUser(ctx, &userID); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	return out, nil
}

func (s *Service) EnableClientUser(ctx context.Context, clientID, userID uuid.UUID) (sqlcdb.ClientUser, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)
	user, err := loadClientUserForClient(ctx, q, clientID, userID)
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if user.Status == sqlcdb.UserStatusActive {
		if err := tx.Commit(ctx); err != nil {
			return sqlcdb.ClientUser{}, err
		}
		return user, nil
	}
	out, err := q.UpdateClientUserStatus(ctx, sqlcdb.UpdateClientUserStatusParams{
		Status: sqlcdb.UserStatusActive,
		ID:     userID,
	})
	if err != nil {
		return sqlcdb.ClientUser{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.ClientUser{}, err
	}
	return out, nil
}

func loadClientUserForClient(ctx context.Context, q *sqlcdb.Queries, clientID, userID uuid.UUID) (sqlcdb.ClientUser, error) {
	cl, err := q.GetClientByIDForUpdate(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.ClientUser{}, ErrNotFound
		}
		return sqlcdb.ClientUser{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return sqlcdb.ClientUser{}, ErrNotFound
	}
	user, err := q.GetClientUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.ClientUser{}, ErrNotFound
		}
		return sqlcdb.ClientUser{}, err
	}
	if user.ClientID != clientID {
		return sqlcdb.ClientUser{}, ErrNotFound
	}
	return user, nil
}

func isUniqueViolation(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == pgerrcode.UniqueViolation
}

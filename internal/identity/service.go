package identity

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/password"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrAccountDisabled    = errors.New("account disabled")
	ErrClientSuspended    = errors.New("client suspended")
	ErrSessionInvalid     = errors.New("session invalid")
	ErrEmailTaken         = errors.New("email already in use")
	ErrValidation         = errors.New("validation")
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
)

type Service struct {
	store *db.Store
	ttl   time.Duration
}

func New(store *db.Store, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	return &Service{store: store, ttl: ttl}
}

type LoginInput struct {
	Email     string
	Password  string
	IP        *netip.Addr
	UserAgent *string
}

type AdminSession struct {
	Session sqlcdb.Session
	User    sqlcdb.AdminUser
}

type ClientSession struct {
	Session sqlcdb.Session
	User    sqlcdb.ClientUser
	Client  sqlcdb.Client
}

func (s *Service) LoginAdmin(ctx context.Context, in LoginInput) (AdminSession, error) {
	email := normalizeEmail(in.Email)
	u, err := s.store.Queries.GetAdminUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminSession{}, ErrInvalidCredentials
		}
		return AdminSession{}, err
	}
	ok, err := password.Verify(in.Password, u.PasswordHash)
	if err != nil || !ok {
		return AdminSession{}, ErrInvalidCredentials
	}
	if u.Status != sqlcdb.UserStatusActive {
		return AdminSession{}, ErrAccountDisabled
	}
	sess, err := s.store.Queries.InsertSession(ctx, sqlcdb.InsertSessionParams{
		Audience:     sqlcdb.SessionAudienceAdmin,
		AdminUserID:  &u.ID,
		ClientUserID: nil,
		ExpiresAt:    time.Now().UTC().Add(s.ttl),
		Ip:           in.IP,
		UserAgent:    in.UserAgent,
	})
	if err != nil {
		return AdminSession{}, err
	}
	return AdminSession{Session: sess, User: u}, nil
}

func (s *Service) LoginClient(ctx context.Context, in LoginInput) (ClientSession, error) {
	email := normalizeEmail(in.Email)
	u, err := s.store.Queries.GetClientUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClientSession{}, ErrInvalidCredentials
		}
		return ClientSession{}, err
	}
	ok, err := password.Verify(in.Password, u.PasswordHash)
	if err != nil || !ok {
		return ClientSession{}, ErrInvalidCredentials
	}
	if u.Status != sqlcdb.UserStatusActive {
		return ClientSession{}, ErrAccountDisabled
	}
	cl, err := s.store.Queries.GetClientByID(ctx, u.ClientID)
	if err != nil {
		return ClientSession{}, err
	}
	if cl.Status != sqlcdb.ClientStatusActive {
		return ClientSession{}, ErrClientSuspended
	}
	sess, err := s.store.Queries.InsertSession(ctx, sqlcdb.InsertSessionParams{
		Audience:     sqlcdb.SessionAudienceClient,
		AdminUserID:  nil,
		ClientUserID: &u.ID,
		ExpiresAt:    time.Now().UTC().Add(s.ttl),
		Ip:           in.IP,
		UserAgent:    in.UserAgent,
	})
	if err != nil {
		return ClientSession{}, err
	}
	return ClientSession{Session: sess, User: u, Client: cl}, nil
}

func (s *Service) Logout(ctx context.Context, sessionID uuid.UUID) error {
	return s.store.Queries.RevokeSession(ctx, sessionID)
}

type Principal struct {
	Audience     sqlcdb.SessionAudience
	SessionID    uuid.UUID
	AdminUserID  *uuid.UUID
	ClientUserID *uuid.UUID
	ClientID     *uuid.UUID
	APIKeyID     *uuid.UUID
	Scopes       []string
	Email        string
	Name         string
	Role         string
	LastSeenAt   time.Time
}

func (p Principal) HasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func (p Principal) AuditActor() (sqlcdb.ActorType, *uuid.UUID) {
	if p.APIKeyID != nil {
		return sqlcdb.ActorTypeApiKey, p.APIKeyID
	}
	if p.AdminUserID != nil {
		return sqlcdb.ActorTypeAdmin, p.AdminUserID
	}
	return sqlcdb.ActorTypeClientUser, p.ClientUserID
}

func (s *Service) Resolve(ctx context.Context, sessionID uuid.UUID, want sqlcdb.SessionAudience) (Principal, error) {
	sess, err := s.store.Queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, ErrSessionInvalid
		}
		return Principal{}, err
	}
	if sess.RevokedAt != nil || time.Now().UTC().After(sess.ExpiresAt) {
		return Principal{}, ErrSessionInvalid
	}
	if sess.Audience != want {
		return Principal{}, ErrSessionInvalid
	}

	p := Principal{
		Audience:     sess.Audience,
		SessionID:    sess.ID,
		AdminUserID:  sess.AdminUserID,
		ClientUserID: sess.ClientUserID,
		LastSeenAt:   sess.LastSeenAt,
	}

	switch sess.Audience {
	case sqlcdb.SessionAudienceAdmin:
		if sess.AdminUserID == nil {
			return Principal{}, ErrSessionInvalid
		}
		u, err := s.store.Queries.GetAdminUserByID(ctx, *sess.AdminUserID)
		if err != nil {
			return Principal{}, ErrSessionInvalid
		}
		if u.Status != sqlcdb.UserStatusActive {
			return Principal{}, ErrAccountDisabled
		}
		p.Email = u.Email
		p.Name = u.Name
		p.Role = "admin"
	case sqlcdb.SessionAudienceClient:
		if sess.ClientUserID == nil {
			return Principal{}, ErrSessionInvalid
		}
		u, err := s.store.Queries.GetClientUserByID(ctx, *sess.ClientUserID)
		if err != nil {
			return Principal{}, ErrSessionInvalid
		}
		if u.Status != sqlcdb.UserStatusActive {
			return Principal{}, ErrAccountDisabled
		}
		cl, err := s.store.Queries.GetClientByID(ctx, u.ClientID)
		if err != nil {
			return Principal{}, ErrSessionInvalid
		}
		if cl.Status != sqlcdb.ClientStatusActive {
			return Principal{}, ErrClientSuspended
		}
		p.Email = u.Email
		p.Name = cl.Name
		p.Role = string(u.Role)
		p.ClientID = &u.ClientID
	default:
		return Principal{}, ErrSessionInvalid
	}

	if time.Since(sess.LastSeenAt) > time.Minute {
		_ = s.store.Queries.TouchSession(ctx, sqlcdb.TouchSessionParams{
			ExpiresAt: time.Now().UTC().Add(s.ttl),
			ID:        sess.ID,
		})
	}
	return p, nil
}

type CreateClientInput struct {
	Name          string
	OwnerEmail    string
	OwnerPassword string
	CreatedBy     uuid.UUID
}

type CreatedClient struct {
	Client sqlcdb.Client
	Owner  sqlcdb.ClientUser
}

func (s *Service) CreateClient(ctx context.Context, in CreateClientInput) (CreatedClient, error) {
	name := strings.TrimSpace(in.Name)
	email := normalizeEmail(in.OwnerEmail)
	if name == "" {
		return CreatedClient{}, fmt.Errorf("%w: name required", ErrValidation)
	}
	if err := validateEmail(email); err != nil {
		return CreatedClient{}, err
	}
	if len(in.OwnerPassword) < 10 {
		return CreatedClient{}, fmt.Errorf("%w: password must be at least 10 characters", ErrValidation)
	}

	if _, err := s.store.Queries.GetClientUserByEmail(ctx, email); err == nil {
		return CreatedClient{}, ErrEmailTaken
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return CreatedClient{}, err
	}

	hash, err := password.Hash(in.OwnerPassword)
	if err != nil {
		return CreatedClient{}, err
	}

	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return CreatedClient{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	cl, err := q.InsertClient(ctx, name)
	if err != nil {
		return CreatedClient{}, err
	}
	if _, err := q.InsertWallet(ctx, sqlcdb.InsertWalletParams{ClientID: cl.ID, Currency: "RUB"}); err != nil {
		return CreatedClient{}, err
	}
	owner, err := q.InsertClientUser(ctx, sqlcdb.InsertClientUserParams{
		ClientID:     cl.ID,
		Email:        email,
		PasswordHash: hash,
	})
	if err != nil {
		return CreatedClient{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedClient{}, err
	}
	return CreatedClient{Client: cl, Owner: owner}, nil
}

func (s *Service) ListClients(ctx context.Context, status *sqlcdb.ClientStatus, limit, offset int32) ([]sqlcdb.ListClientsRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	arg := sqlcdb.ListClientsParams{PageLimit: limit, PageOffset: offset}
	if status != nil {
		arg.Status = sqlcdb.NullClientStatus{ClientStatus: *status, Valid: true}
	}
	return s.store.Queries.ListClients(ctx, arg)
}

type ClientDetail struct {
	Client sqlcdb.Client
	Users  []sqlcdb.ClientUser
}

func (s *Service) GetClient(ctx context.Context, id uuid.UUID) (ClientDetail, error) {
	cl, err := s.store.Queries.GetClientByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ClientDetail{}, ErrNotFound
		}
		return ClientDetail{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return ClientDetail{}, ErrNotFound
	}
	users, err := s.store.Queries.ListClientUsersByClientID(ctx, id)
	if err != nil {
		return ClientDetail{}, err
	}
	return ClientDetail{Client: cl, Users: users}, nil
}

func (s *Service) UpdateClient(ctx context.Context, id uuid.UUID, name string) (sqlcdb.Client, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return sqlcdb.Client{}, fmt.Errorf("%w: name required", ErrValidation)
	}
	cl, err := s.store.Queries.UpdateClientName(ctx, sqlcdb.UpdateClientNameParams{Name: name, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.Client{}, ErrNotFound
		}
		return sqlcdb.Client{}, err
	}
	return cl, nil
}

func (s *Service) SuspendClient(ctx context.Context, id uuid.UUID) (sqlcdb.Client, error) {
	return s.setStatus(ctx, id, sqlcdb.ClientStatusActive, sqlcdb.ClientStatusSuspended)
}

func (s *Service) ActivateClient(ctx context.Context, id uuid.UUID) (sqlcdb.Client, error) {
	return s.setStatus(ctx, id, sqlcdb.ClientStatusSuspended, sqlcdb.ClientStatusActive)
}

func (s *Service) DeleteClient(ctx context.Context, id uuid.UUID) (sqlcdb.Client, error) {
	return s.DeleteClientAnd(ctx, id, nil)
}

func (s *Service) DeleteClientAnd(ctx context.Context, id uuid.UUID, afterLock func(context.Context, *sqlcdb.Queries) error) (sqlcdb.Client, error) {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return sqlcdb.Client{}, err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	cl, err := q.GetClientByIDForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.Client{}, ErrNotFound
		}
		return sqlcdb.Client{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return sqlcdb.Client{}, ErrNotFound
	}
	if afterLock != nil {
		if err := afterLock(ctx, q); err != nil {
			return sqlcdb.Client{}, err
		}
	}
	out, err := q.SetClientStatus(ctx, sqlcdb.SetClientStatusParams{
		Status: sqlcdb.ClientStatusDeleted,
		ID:     id,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.Client{}, ErrNotFound
		}
		return sqlcdb.Client{}, err
	}
	if err := q.RevokeSessionsForClient(ctx, id); err != nil {
		return sqlcdb.Client{}, err
	}
	if err := q.RevokeAPICredentialsForClient(ctx, id); err != nil {
		return sqlcdb.Client{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return sqlcdb.Client{}, err
	}
	return out, nil
}

func (s *Service) setStatus(ctx context.Context, id uuid.UUID, from, to sqlcdb.ClientStatus) (sqlcdb.Client, error) {
	cl, err := s.store.Queries.GetClientByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.Client{}, ErrNotFound
		}
		return sqlcdb.Client{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return sqlcdb.Client{}, ErrNotFound
	}
	if cl.Status != from {
		return sqlcdb.Client{}, fmt.Errorf("%w: client is %s", ErrConflict, cl.Status)
	}
	out, err := s.store.Queries.SetClientStatus(ctx, sqlcdb.SetClientStatusParams{Status: to, ID: id})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlcdb.Client{}, ErrNotFound
		}
		return sqlcdb.Client{}, err
	}
	if to == sqlcdb.ClientStatusSuspended {
		_ = s.store.Queries.RevokeSessionsForClient(ctx, id)
	}
	return out, nil
}

func (s *Service) ResetOwnerPassword(ctx context.Context, clientID uuid.UUID, newPassword string) error {
	if len(newPassword) < 10 {
		return fmt.Errorf("%w: password must be at least 10 characters", ErrValidation)
	}
	cl, err := s.store.Queries.GetClientByID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return ErrNotFound
	}
	owner, err := s.store.Queries.GetOwnerByClientID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
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
	if err := q.UpdateClientUserPassword(ctx, sqlcdb.UpdateClientUserPasswordParams{
		PasswordHash: hash,
		ID:           owner.ID,
	}); err != nil {
		return err
	}
	if err := q.RevokeSessionsForClient(ctx, clientID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func validateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("%w: email required", ErrValidation)
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return fmt.Errorf("%w: invalid email", ErrValidation)
	}
	return nil
}

package apikeys

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/identity"
)

var (
	ErrInvalidToken = errors.New("invalid api key")
	ErrRevoked      = errors.New("api key revoked")
	ErrCIDR         = errors.New("ip not allowed")
	ErrValidation   = errors.New("validation")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

const (
	ScopeSend        = "sms:send"
	ScopeRead        = "sms:read"
	ScopeCampaigns   = "campaigns:write"
	ScopeLookupWrite = "lookup:write"
	ScopeLookupRead  = "lookup:read"
)

var allScopes = []string{ScopeSend, ScopeRead, ScopeCampaigns, ScopeLookupWrite, ScopeLookupRead}

type Service struct {
	store  *db.Store
	pepper string
}

func New(store *db.Store, pepper string) *Service {
	return &Service{store: store, pepper: pepper}
}

type CreateInput struct {
	ClientID     uuid.UUID
	Name         string
	Scopes       []string
	AllowedCIDRs []string
	CreatedBy    uuid.UUID
}

type Public struct {
	ID           uuid.UUID
	ClientID     uuid.UUID
	Name         string
	KeyPrefix    string
	Scopes       []string
	Status       sqlcdb.CredentialStatus
	AllowedCIDRs []string
	LastUsedAt   *time.Time
	CreatedAt    time.Time
}

type Created struct {
	Public
	Token string
}

func (s *Service) Create(ctx context.Context, in CreateInput) (Created, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Created{}, fmt.Errorf("%w: name required", ErrValidation)
	}
	scopes, err := normalizeScopes(in.Scopes)
	if err != nil {
		return Created{}, err
	}
	cidrs, err := ParseCIDRs(in.AllowedCIDRs)
	if err != nil {
		return Created{}, err
	}
	cl, err := s.store.Queries.GetClientByID(ctx, in.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Created{}, ErrNotFound
		}
		return Created{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return Created{}, ErrNotFound
	}

	var last error
	for i := 0; i < 5; i++ {
		prefix, secret, err := generate()
		if err != nil {
			return Created{}, err
		}
		row, err := s.store.Queries.InsertAPICredential(ctx, sqlcdb.InsertAPICredentialParams{
			ClientID:     in.ClientID,
			Name:         name,
			KeyPrefix:    prefix,
			SecretHash:   HashSecret(s.pepper, secret),
			Scopes:       scopes,
			AllowedCidrs: cidrs,
			CreatedBy:    &in.CreatedBy,
		})
		if err != nil {
			if isUniqueViolation(err) {
				last = err
				continue
			}
			return Created{}, err
		}
		return Created{
			Public: publicFromInsert(row),
			Token:  Format(prefix, secret),
		}, nil
	}
	return Created{}, fmt.Errorf("create api key: %w", last)
}

func (s *Service) List(ctx context.Context, clientID uuid.UUID) ([]Public, error) {
	cl, err := s.store.Queries.GetClientByID(ctx, clientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return nil, ErrNotFound
	}
	rows, err := s.store.Queries.ListAPICredentialsForClient(ctx, clientID)
	if err != nil {
		return nil, err
	}
	out := make([]Public, 0, len(rows))
	for _, r := range rows {
		out = append(out, Public{
			ID:           r.ID,
			ClientID:     r.ClientID,
			Name:         r.Name,
			KeyPrefix:    r.KeyPrefix,
			Scopes:       r.Scopes,
			Status:       r.Status,
			AllowedCIDRs: r.AllowedCidrs,
			LastUsedAt:   r.LastUsedAt,
			CreatedAt:    r.CreatedAt,
		})
	}
	return out, nil
}

func (s *Service) Revoke(ctx context.Context, clientID, keyID uuid.UUID) (Public, error) {
	row, err := s.store.Queries.RevokeAPICredential(ctx, sqlcdb.RevokeAPICredentialParams{
		ID:       keyID,
		ClientID: clientID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			cur, getErr := s.store.Queries.GetAPICredentialByIDForClient(ctx, sqlcdb.GetAPICredentialByIDForClientParams{
				ID:       keyID,
				ClientID: clientID,
			})
			if getErr != nil {
				if errors.Is(getErr, pgx.ErrNoRows) {
					return Public{}, ErrNotFound
				}
				return Public{}, getErr
			}
			if cur.Status == sqlcdb.CredentialStatusRevoked {
				return Public{}, fmt.Errorf("%w: already revoked", ErrConflict)
			}
			return Public{}, ErrNotFound
		}
		return Public{}, err
	}
	return Public{
		ID:           row.ID,
		ClientID:     row.ClientID,
		Name:         row.Name,
		KeyPrefix:    row.KeyPrefix,
		Scopes:       row.Scopes,
		Status:       row.Status,
		AllowedCIDRs: row.AllowedCidrs,
		LastUsedAt:   row.LastUsedAt,
		CreatedAt:    row.CreatedAt,
	}, nil
}

func (s *Service) Resolve(ctx context.Context, token string, ip *netip.Addr) (identity.Principal, error) {
	parsed, err := Parse(token)
	if err != nil {
		return identity.Principal{}, ErrInvalidToken
	}
	row, err := s.store.Queries.GetAPICredentialByPrefix(ctx, parsed.Prefix)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.Principal{}, ErrInvalidToken
		}
		return identity.Principal{}, err
	}
	if !Verify(s.pepper, parsed.Secret, row.SecretHash) {
		return identity.Principal{}, ErrInvalidToken
	}
	if row.Status != sqlcdb.CredentialStatusActive {
		return identity.Principal{}, ErrRevoked
	}
	if !IPAllowed(row.AllowedCidrs, ip) {
		return identity.Principal{}, ErrCIDR
	}
	cl, err := s.store.Queries.GetClientByID(ctx, row.ClientID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return identity.Principal{}, ErrInvalidToken
		}
		return identity.Principal{}, err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return identity.Principal{}, ErrInvalidToken
	}
	if cl.Status == sqlcdb.ClientStatusSuspended {
		return identity.Principal{}, identity.ErrClientSuspended
	}
	_ = s.store.Queries.TouchAPICredential(ctx, row.ID)
	return identity.Principal{
		ClientID: &row.ClientID,
		APIKeyID: &row.ID,
		Scopes:   row.Scopes,
		Name:     cl.Name,
		Role:     "api_key",
	}, nil
}

func normalizeScopes(in []string) ([]string, error) {
	if len(in) == 0 {
		out := make([]string, len(allScopes))
		copy(out, allScopes)
		return out, nil
	}
	allowed := make(map[string]struct{}, len(allScopes))
	for _, s := range allScopes {
		allowed[s] = struct{}{}
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := allowed[s]; !ok {
			return nil, fmt.Errorf("%w: unknown scope %q", ErrValidation, s)
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one scope required", ErrValidation)
	}
	return out, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

func (p Public) JSON() map[string]any {
	out := map[string]any{
		"id":            p.ID,
		"client_id":     p.ClientID,
		"name":          p.Name,
		"key_prefix":    p.KeyPrefix,
		"scopes":        p.Scopes,
		"status":        p.Status,
		"allowed_cidrs": p.AllowedCIDRs,
		"created_at":    p.CreatedAt.UTC().Format(time.RFC3339),
	}
	if p.AllowedCIDRs == nil {
		out["allowed_cidrs"] = []string{}
	}
	if p.LastUsedAt != nil {
		out["last_used_at"] = p.LastUsedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func publicFromInsert(row sqlcdb.InsertAPICredentialRow) Public {
	return Public{
		ID:           row.ID,
		ClientID:     row.ClientID,
		Name:         row.Name,
		KeyPrefix:    row.KeyPrefix,
		Scopes:       row.Scopes,
		Status:       row.Status,
		AllowedCIDRs: row.AllowedCidrs,
		LastUsedAt:   row.LastUsedAt,
		CreatedAt:    row.CreatedAt,
	}
}

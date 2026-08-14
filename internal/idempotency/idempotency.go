package idempotency

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const DefaultTTL = 24 * time.Hour

var (
	ErrConflict = errors.New("idempotency key reused with a different request")
	ErrInFlight = errors.New("idempotency key is already in flight")
)

type Record struct {
	ID     uuid.UUID
	Replay bool
	Status int
	Body   []byte
}

func HashRequest(method, path string, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(method))
	_, _ = h.Write([]byte{'\n'})
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte{'\n'})
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func Reserve(ctx context.Context, q *sqlcdb.Queries, principalType sqlcdb.ActorType, principalID uuid.UUID, key, requestHash string, ttl time.Duration) (Record, error) {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	row, err := q.InsertIdempotencyKey(ctx, sqlcdb.InsertIdempotencyKeyParams{
		PrincipalType: principalType,
		PrincipalID:   principalID,
		Key:           key,
		RequestHash:   requestHash,
		ExpiresAt:     time.Now().UTC().Add(ttl),
	})
	if err == nil {
		return Record{ID: row.ID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Record{}, err
	}
	existing, err := q.GetIdempotencyKeyForUpdate(ctx, sqlcdb.GetIdempotencyKeyForUpdateParams{
		PrincipalType: principalType,
		PrincipalID:   principalID,
		Key:           key,
	})
	if err != nil {
		return Record{}, err
	}
	if existing.RequestHash != requestHash {
		return Record{}, ErrConflict
	}
	if existing.ResponseStatus != nil {
		return Record{
			ID:     existing.ID,
			Replay: true,
			Status: int(*existing.ResponseStatus),
			Body:   existing.ResponseBody,
		}, nil
	}
	return Record{}, ErrInFlight
}

func Complete(ctx context.Context, q *sqlcdb.Queries, id uuid.UUID, status int, body []byte) error {
	st := int32(status)
	_, err := q.CompleteIdempotencyKey(ctx, sqlcdb.CompleteIdempotencyKeyParams{
		ResponseStatus: &st,
		ResponseBody:   body,
		ID:             id,
	})
	return err
}

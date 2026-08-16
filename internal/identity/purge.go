package identity

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const (
	scrambledEmailPrefix = "deleted-"
	scrambledEmailSuffix = "@invalid"
	PurgeHTTPBudget      = 15 * time.Second
	purgeWorkerBatches   = 3
	purgePendingLimit    = 4
	purgeLeakScanLimit   = 200
)

func ScrambledUserEmail(userID uuid.UUID) string {
	return scrambledEmailPrefix + userID.String() + scrambledEmailSuffix
}

func EmailAlreadyScrambled(email string) bool {
	e := strings.ToLower(strings.TrimSpace(email))
	return strings.HasPrefix(e, scrambledEmailPrefix) && strings.HasSuffix(e, scrambledEmailSuffix)
}

func purgeLockKeys(id uuid.UUID) sqlcdb.TryAdvisoryLockClientPurgeParams {
	return sqlcdb.TryAdvisoryLockClientPurgeParams{
		Key1: int32(binary.BigEndian.Uint32(id[0:4])),
		Key2: int32(binary.BigEndian.Uint32(id[4:8])),
	}
}

type DeleteClientResult struct {
	Client sqlcdb.Client
	Fresh  bool
}

func (s *Service) DeleteClient(ctx context.Context, id uuid.UUID) (sqlcdb.Client, error) {
	out, err := s.DeleteClientAnd(ctx, id, nil)
	return out.Client, err
}

func (s *Service) DeleteClientAnd(ctx context.Context, id uuid.UUID, afterLock func(context.Context, *sqlcdb.Queries) error) (DeleteClientResult, error) {
	cl, err := s.store.Queries.GetClientByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DeleteClientResult{}, ErrNotFound
		}
		return DeleteClientResult{}, err
	}

	fresh := cl.Status != sqlcdb.ClientStatusDeleted
	if !fresh {
		leftover, err := s.store.Queries.ClientHasPurgeLeftover(ctx, id)
		if err != nil {
			return DeleteClientResult{}, err
		}
		if !leftover {
			return DeleteClientResult{}, ErrNotFound
		}
	} else {
		if err := s.markDeleted(ctx, id, afterLock); err != nil {
			return DeleteClientResult{}, err
		}
		cl, err = s.store.Queries.GetClientByID(ctx, id)
		if err != nil {
			return DeleteClientResult{}, err
		}
	}

	_ = s.purgeClient(ctx, id)
	return DeleteClientResult{Client: cl, Fresh: fresh}, nil
}

func (s *Service) markDeleted(ctx context.Context, id uuid.UUID, afterLock func(context.Context, *sqlcdb.Queries) error) error {
	tx, err := s.store.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.store.Queries.WithTx(tx)

	cl, err := q.GetClientByIDForUpdate(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if cl.Status == sqlcdb.ClientStatusDeleted {
		return tx.Commit(ctx)
	}
	if afterLock != nil {
		if err := afterLock(ctx, q); err != nil {
			return err
		}
	}
	if _, err := q.MarkClientDeleted(ctx, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if err := q.RevokeSessionsForClient(ctx, id); err != nil {
		return err
	}
	if err := q.RevokeAPICredentialsForClient(ctx, id); err != nil {
		return err
	}
	if _, err := q.CancelOpenCampaignsForClient(ctx, id); err != nil {
		return err
	}
	if _, err := q.DisableWebhookEndpointsForClient(ctx, id); err != nil {
		return err
	}
	if _, err := q.ScrambleClientUsersForDelete(ctx, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) PurgeTick(ctx context.Context) (int, error) {
	if s == nil || s.store == nil {
		return 0, nil
	}
	ids, err := s.store.Queries.ListClientsPendingPurge(ctx, purgePendingLimit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, id := range ids {
		deleted, err := s.purgeClientBatches(ctx, id, purgeWorkerBatches)
		if err != nil {
			return n, err
		}
		n += deleted
	}
	return n, nil
}

func (s *Service) ReopenLeakedPurges(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	ids, err := s.store.Queries.ListPurgedDeletedClientIDs(ctx, purgeLeakScanLimit)
	if err != nil {
		return err
	}
	for _, id := range ids {
		leftover, err := s.store.Queries.ClientHasPurgeLeftover(ctx, id)
		if err != nil {
			return err
		}
		if leftover {
			if err := s.store.Queries.ClearClientPurgedAt(ctx, id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) purgeClient(ctx context.Context, id uuid.UUID) error {
	for ctx.Err() == nil {
		n, err := s.purgeClientBatches(ctx, id, 8)
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		return nil
	}
	return nil
}

func (s *Service) purgeClientBatches(ctx context.Context, id uuid.UUID, maxBatches int) (int, error) {
	conn, err := s.store.Pool.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer conn.Release()
	q := sqlcdb.New(conn)
	keys := purgeLockKeys(id)
	locked, err := q.TryAdvisoryLockClientPurge(ctx, keys)
	if err != nil {
		return 0, err
	}
	if !locked {
		return 0, nil
	}
	defer func() {
		_, _ = q.AdvisoryUnlockClientPurge(context.WithoutCancel(ctx), sqlcdb.AdvisoryUnlockClientPurgeParams{
			Key1: keys.Key1,
			Key2: keys.Key2,
		})
	}()

	total := 0
	for i := 0; i < maxBatches; i++ {
		if ctx.Err() != nil {
			return total, nil
		}
		n, err := purgeOneRound(ctx, q, id)
		if err != nil {
			return total, err
		}
		total += int(n)
		if n == 0 {
			leftover, err := q.ClientHasPurgeLeftover(ctx, id)
			if err != nil {
				return total, err
			}
			if !leftover {
				if err := q.MarkClientPurged(ctx, id); err != nil {
					return total, err
				}
			}
			return total, nil
		}
	}
	return total, nil
}

func purgeByCIDPtr(fn func(context.Context, *uuid.UUID) (int64, error)) func(context.Context, uuid.UUID) (int64, error) {
	return func(ctx context.Context, id uuid.UUID) (int64, error) {
		return fn(ctx, &id)
	}
}

func purgeOneRound(ctx context.Context, q *sqlcdb.Queries, id uuid.UUID) (int64, error) {
	steps := []func(context.Context, uuid.UUID) (int64, error){
		q.PurgeWebhookDeliveriesForClient,
		q.PurgeWebhookEndpointsForClient,
		purgeByCIDPtr(q.PurgeProviderLookupCallbacksForClient),
		purgeByCIDPtr(q.PurgeProviderLookupRequestsForClient),
		q.PurgeLookupCSVPreviewsForClient,
		q.PurgeWalletTransactionsForClient,
		q.PurgeLookupItemsForClient,
		q.PurgeLookupJobsForClient,
		purgeByCIDPtr(q.PurgeSendJobsForClient),
		purgeByCIDPtr(q.PurgeCallbackEventsForClientMessages),
		func(ctx context.Context, id uuid.UUID) (int64, error) {
			n, err := q.UnlinkCampaignRecipientsForClient(ctx, &id)
			if err != nil {
				return 0, err
			}
			m, err := q.PurgeSmsMessagesForClient(ctx, &id)
			return n + m, err
		},
		q.PurgeSmsCampaignsForClient,
		func(ctx context.Context, id uuid.UUID) (int64, error) {
			n, err := q.PurgeClientTariffsForClient(ctx, id)
			if err != nil {
				return 0, err
			}
			m, err := q.PurgeWalletsForClient(ctx, id)
			return n + m, err
		},
		func(ctx context.Context, id uuid.UUID) (int64, error) {
			n, err := q.UnlinkDirectionJobsForClient(ctx, id)
			if err != nil {
				return 0, err
			}
			m, err := q.PurgeNumberAssignmentsForClient(ctx, id)
			return n + m, err
		},
		purgeByCIDPtr(q.PurgeAuditLogForClient),
		purgeByCIDPtr(q.PurgeOpsEventsForClient),
		q.PurgeIdempotencyForClient,
		q.PurgeSessionsForClient,
		q.PurgeAPICredentialsForClient,
		q.PurgeClientUsersForClient,
	}
	for _, step := range steps {
		n, err := step(ctx, id)
		if err != nil {
			return 0, err
		}
		if n > 0 {
			return n, nil
		}
	}
	return 0, nil
}

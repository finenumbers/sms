package campaigns

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/messaging"
	"finenumbers/sms/internal/msisdn"
)

const (
	fanoutClients    = 8
	fanoutPerClient  = 100
	claimQueuedLimit = 20
	aggregateEvery   = 15 * time.Second
	counterLookback  = 7 * 24 * time.Hour
)

type skipReason uint8

const (
	skipNone skipReason = iota
	skipNotRunning
	skipUnassigned
)

type Worker struct {
	store   *db.Store
	dirs    DirectionPolicy
	msg     *messaging.Service
	log     *slog.Logger
	now     func() time.Time
	lastAgg time.Time
}

func NewWorker(store *db.Store, dirs DirectionPolicy, msg *messaging.Service, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		store: store,
		dirs:  dirs,
		msg:   msg,
		log:   log,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (w *Worker) Tick(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	if _, err := w.store.Queries.SkipPendingForCancelledCampaigns(ctx); err != nil && w.log != nil {
		w.log.Error("drain cancelled recipients", "err", err)
	}
	queued, err := w.store.Queries.ClaimQueuedCampaigns(ctx, claimQueuedLimit)
	if err != nil {
		return 0, err
	}
	n, err := w.fanout(ctx)
	if err != nil {
		return len(queued) + n, err
	}
	if w.lastAgg.IsZero() || w.now().Sub(w.lastAgg) >= aggregateEvery {
		if err := w.store.Queries.RefreshCampaignCounters(ctx, w.now().Add(-counterLookback)); err != nil && w.log != nil {
			w.log.Error("campaign counters", "err", err)
		}
		if _, err := w.store.Queries.CompleteIdleCampaigns(ctx); err != nil && w.log != nil {
			w.log.Error("complete campaigns", "err", err)
		}
		w.lastAgg = w.now()
	}
	return len(queued) + n, nil
}

func (w *Worker) fanout(ctx context.Context) (int, error) {
	clients, err := w.store.Queries.ListFanoutClientIDs(ctx, fanoutClients)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, cid := range clients {
		n, err := w.fanoutClient(ctx, cid)
		if err != nil {
			if w.log != nil {
				w.log.Error("campaign fan-out client", "client_id", cid, "err", err)
			}
			continue
		}
		total += n
	}
	return total, nil
}

func (w *Worker) fanoutClient(ctx context.Context, clientID uuid.UUID) (int, error) {
	tx, err := w.store.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	q := w.store.Queries.WithTx(tx)

	rows, err := q.LockPendingRecipientsForClient(ctx, sqlcdb.LockPendingRecipientsForClientParams{
		ClientID:  clientID,
		PerClient: fanoutPerClient,
	})
	if err != nil {
		return 0, err
	}
	intOut := false
	if w.dirs != nil {
		if v, err := w.dirs.Get(ctx); err == nil {
			intOut = v.SMSDirections.IntOut
		}
	}
	sticky := map[uuid.UUID]skipReason{}
	for _, r := range rows {
		if reason := sticky[r.CampaignID]; reason != skipNone {
			_ = q.MarkRecipientSkipped(ctx, r.ID)
			continue
		}
		ok, err := w.stillRunnable(ctx, q, r, sticky)
		if err != nil {
			return 0, err
		}
		if !ok {
			_ = q.MarkRecipientSkipped(ctx, r.ID)
			continue
		}
		dest, err := msisdn.NormalizeDest(r.ToMsisdn)
		if err != nil {
			_ = q.MarkRecipientFailed(ctx, r.ID)
			continue
		}
		if dest.International && !intOut {
			_ = q.MarkRecipientSkipped(ctx, r.ID)
			continue
		}
		if w.msg == nil {
			return 0, errors.New("messaging service required")
		}
		camp := r.CampaignID
		if _, err := tx.Exec(ctx, "SAVEPOINT recip"); err != nil {
			return 0, err
		}
		msg, err := w.msg.EnqueueWith(ctx, q, messaging.EnqueueInput{
			ClientID:   r.ClientID,
			From:       r.FromMsisdn,
			To:         dest.MSISDN,
			Text:       r.Text,
			CampaignID: &camp,
		})
		if err != nil {
			_, _ = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT recip")
			if errors.Is(err, billing.ErrInsufficientFunds) || errors.Is(err, billing.ErrTariffNotConfigured) || errors.Is(err, billing.ErrInvalidTariff) {
				_ = q.MarkRecipientFailed(ctx, r.ID)
				continue
			}
			return 0, err
		}
		if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT recip"); err != nil {
			return 0, err
		}
		mid := msg.ID
		if err := q.MarkRecipientEnqueued(ctx, sqlcdb.MarkRecipientEnqueuedParams{
			SmsMessageID: &mid,
			ID:           r.ID,
		}); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(rows), nil
}

func (w *Worker) stillRunnable(ctx context.Context, q *sqlcdb.Queries, r sqlcdb.LockPendingRecipientsForClientRow, sticky map[uuid.UUID]skipReason) (bool, error) {
	row, err := q.CampaignRunnableForFanout(ctx, r.CampaignID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sticky[r.CampaignID] = skipNotRunning
			return false, nil
		}
		return false, err
	}
	if !row.Running {
		sticky[r.CampaignID] = skipNotRunning
		return false, nil
	}
	if !row.Assigned {
		sticky[r.CampaignID] = skipUnassigned
		if w.log != nil {
			w.log.Error("campaign from unassigned", "campaign_id", r.CampaignID)
		}
		return false, nil
	}
	return true, nil
}

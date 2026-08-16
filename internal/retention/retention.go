package retention

import (
	"context"
	"log/slog"
	"time"

	"finenumbers/sms/internal/db"
	"finenumbers/sms/internal/metrics"
	"finenumbers/sms/internal/settings"
)

const (
	interval    = time.Hour
	maxLoops    = 50
	maxOpsLoops = 200
)

type SettingsView interface {
	Get(ctx context.Context) (settings.Public, error)
}

type ClientPurge interface {
	PurgeTick(ctx context.Context) (int, error)
	ReopenLeakedPurges(ctx context.Context) error
}

type Worker struct {
	store    *db.Store
	settings SettingsView
	purge    ClientPurge
	log      *slog.Logger
	now      func() time.Time
	last     time.Time
}

func NewWorker(store *db.Store, settings SettingsView, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{store: store, settings: settings, log: log, now: func() time.Time { return time.Now().UTC() }}
}

func (w *Worker) SetClientPurge(p ClientPurge) {
	if w != nil {
		w.purge = p
	}
}

func (w *Worker) Tick(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if w.purge != nil {
		if _, err := w.purge.PurgeTick(ctx); err != nil {
			return err
		}
	}
	if !w.last.IsZero() && w.now().Sub(w.last) < interval {
		return nil
	}
	if w.store == nil {
		return nil
	}
	if err := w.run(ctx); err != nil {
		return err
	}
	w.last = w.now()
	return nil
}

func (w *Worker) run(ctx context.Context) error {
	view, err := w.settings.Get(ctx)
	if err != nil {
		return err
	}
	now := w.now()
	var msgCut, auditCut, opsCut time.Time
	if days := int(view.RetentionDays); days >= 1 {
		msgCut = now.AddDate(0, 0, -days)
	} else {
		msgCut = now.AddDate(0, 0, -365)
	}
	if days := int(view.AuditRetentionDays); days >= 1 {
		auditCut = now.AddDate(0, 0, -days)
	} else {
		auditCut = now.AddDate(0, 0, -730)
	}
	if days := int(view.OpsRetentionDays); days >= 1 {
		opsCut = now.AddDate(0, 0, -days)
	} else {
		opsCut = now.AddDate(0, 0, -14)
	}

	n, err := w.store.Queries.DeleteExpiredSessions(ctx)
	if err != nil {
		return err
	}
	addDeleted("sessions", n)

	n, err = w.store.Queries.DeleteExpiredIdempotencyKeys(ctx)
	if err != nil {
		return err
	}
	addDeleted("idempotency_keys", n)

	if err := w.loop(ctx, "sms_messages", maxLoops, func(ctx context.Context) (int64, error) {
		return w.store.Queries.DeleteOldSmsMessages(ctx, msgCut)
	}); err != nil {
		return err
	}
	n, err = w.store.Queries.DeleteOldCampaigns(ctx, msgCut)
	if err != nil {
		return err
	}
	addDeleted("sms_campaigns", n)

	if err := w.loop(ctx, "provider_callback_events", maxLoops, func(ctx context.Context) (int64, error) {
		return w.store.Queries.DeleteOldCallbackEvents(ctx, msgCut)
	}); err != nil {
		return err
	}
	if err := w.loop(ctx, "audit_log", maxLoops, func(ctx context.Context) (int64, error) {
		return w.store.Queries.DeleteOldAuditLog(ctx, auditCut)
	}); err != nil {
		return err
	}
	if err := w.loop(ctx, "ops_events", maxOpsLoops, func(ctx context.Context) (int64, error) {
		return w.store.Queries.DeleteOldOpsEvents(ctx, opsCut)
	}); err != nil {
		return err
	}
	if w.purge != nil {
		if err := w.purge.ReopenLeakedPurges(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (w *Worker) loop(ctx context.Context, table string, max int, fn func(context.Context) (int64, error)) error {
	if max < 1 {
		max = 1
	}
	for i := 0; i < max; i++ {
		n, err := fn(ctx)
		if err != nil {
			return err
		}
		addDeleted(table, n)
		if n == 0 {
			return nil
		}
	}
	if w.log != nil {
		w.log.Info("retention batch cap reached", "table", table)
	}
	return nil
}

func addDeleted(table string, n int64) {
	if n > 0 {
		metrics.RetentionDeleted.WithLabelValues(table).Add(float64(n))
	}
}

package lookup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/ratelimit"
	"finenumbers/sms/internal/settings"
	"finenumbers/sms/internal/smsc"
	"finenumbers/sms/internal/webhooks"
)

const (
	PollMaxAttempts = 120
	SubmitBatchSize = 50
	fairClients     = 8
	tickBudget      = 150 * time.Millisecond
	minHTTPBudget   = 20 * time.Millisecond
	smscCallTimeout = smsc.DefaultHTTPTimeout
	reconcileEvery  = 15 * time.Second
	csvShellAge     = 5 * time.Minute
	// Must outlive Create's statement_timeout. Heal-to-ready while Create
	// still holds the TX lets a second cabinet submit take a new HOLD.
	csvConsumingAge   = createStatementTimeout + 3*time.Minute
	EnrichMaxAttempts = 3
	smscRateKey       = "rl:smsc"
)

type Worker struct {
	store    *db.Store
	billing  *billing.Service
	provider Provider
	settings SettingsView
	limiter  *ratelimit.Limiter
	svc      *Service
	hooks    *webhooks.Service
	log      *slog.Logger
	now      func() time.Time
	lastRec  time.Time
	lastBal  time.Time
	cache    *smsc.BalanceCache
	smscBal  func(context.Context) (smsc.Balance, error)
}

func (w *Worker) SetService(s *Service) {
	if w != nil {
		w.svc = s
	}
}

func (w *Worker) SetBalanceRefresh(cache *smsc.BalanceCache, get func(context.Context) (smsc.Balance, error)) {
	if w == nil {
		return
	}
	w.cache = cache
	w.smscBal = get
}

func (w *Worker) SetWebhooks(s *webhooks.Service) {
	if w != nil {
		w.hooks = s
		if w.svc != nil {
			w.svc.SetWebhooks(s)
		}
	}
}

func NewWorker(store *db.Store, bill *billing.Service, provider Provider, settings SettingsView, limiter *ratelimit.Limiter, log *slog.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		store:    store,
		billing:  bill,
		provider: provider,
		settings: settings,
		limiter:  limiter,
		log:      log,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (w *Worker) Tick(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	deadline := w.now().Add(tickBudget)
	n := 0
	if c, err := w.applyStoredCallbacks(ctx); err != nil && w.log != nil {
		w.log.Error("lookup callbacks", "err", err)
	} else {
		n += c
	}
	if w.now().Before(deadline) {
		c, err := w.parsePendingCSV(ctx)
		if err != nil && w.log != nil {
			w.log.Error("lookup csv parse", "err", err)
		} else {
			n += c
		}
	}
	if w.now().Before(deadline) {
		c, err := w.submit(ctx, deadline)
		if err != nil && w.log != nil {
			w.log.Error("lookup submit", "err", err)
		} else {
			n += c
		}
	}
	if w.now().Before(deadline) {
		c, err := w.poll(ctx, deadline)
		if err != nil && w.log != nil {
			w.log.Error("lookup poll", "err", err)
		} else {
			n += c
		}
	}
	if c, err := w.applyStoredCallbacks(ctx); err != nil && w.log != nil {
		w.log.Error("lookup callbacks", "err", err)
	} else {
		n += c
	}
	if w.now().Before(deadline) {
		c, err := w.enrichTerminalHLR(ctx, deadline)
		if err != nil && w.log != nil {
			w.log.Error("lookup hlr enrich", "err", err)
		} else {
			n += c
		}
	}
	if c, err := w.finalizeReady(ctx); err != nil && w.log != nil {
		w.log.Error("lookup finalize", "err", err)
	} else {
		n += c
	}
	if w.now().Before(deadline) {
		c, err := w.deliverWebhooks(ctx)
		if err != nil && w.log != nil {
			w.log.Error("lookup webhooks", "err", err)
		} else {
			n += c
		}
	}
	if w.lastRec.IsZero() || w.now().Sub(w.lastRec) >= reconcileEvery {
		if err := w.reconcile(ctx); err != nil && w.log != nil {
			w.log.Error("lookup reconcile", "err", err)
		}
		w.lastRec = w.now()
	}
	return n, nil
}

// BalanceRefreshEvery is the cadence of the dedicated SMSC balance ticker.
// Kept out of Tick so balance.php cannot stall CSV/submit/poll.
const BalanceRefreshEvery = 2 * time.Minute

func (w *Worker) RefreshSMSCBalance(ctx context.Context) {
	w.refreshSMSCBalance(ctx)
}

func (w *Worker) refreshSMSCBalance(ctx context.Context) {
	if w == nil || w.cache == nil || w.smscBal == nil {
		return
	}
	if w.provider == nil || !w.provider.Configured() {
		return
	}
	if !w.lastBal.IsZero() && w.now().Sub(w.lastBal) < BalanceRefreshEvery {
		return
	}
	bal, err := w.smscBal(ctx)
	if err != nil {
		if w.log != nil {
			w.log.Warn("smsc balance cache", "err", err)
		}
		return
	}
	if err := w.cache.Write(ctx, bal); err != nil && w.log != nil {
		w.log.Warn("smsc balance cache write", "err", err)
		return
	}
	w.lastBal = w.now()
}

func (w *Worker) runtime(ctx context.Context) (settings.Public, error) {
	if w.settings == nil {
		return settings.Public{
			LookupEnabled:            false,
			LookupCheckTimeoutSec:    3600,
			LookupPollIntervalSec:    30,
			LookupMaxBatchPhones:     1000,
			LookupWebhookMaxAttempts: 8,
			LookupWebhookTimeoutMs:   5000,
		}, nil
	}
	return w.settings.Get(ctx)
}

func (w *Worker) submit(ctx context.Context, deadline time.Time) (int, error) {
	view, err := w.runtime(ctx)
	if err != nil {
		return 0, err
	}
	if !view.LookupEnabled {
		return 0, nil
	}
	per := int32(SubmitBatchSize / fairClients)
	if per < 1 {
		per = 1
	}
	queued, err := w.store.Queries.ClaimQueuedLookupItemsFair(ctx, sqlcdb.ClaimQueuedLookupItemsFairParams{
		ClientLimit: fairClients,
		PerClient:   per,
	})
	if err != nil {
		return 0, err
	}
	reserved, err := w.store.Queries.ClaimReservedLookupItemsFair(ctx, sqlcdb.ClaimReservedLookupItemsFairParams{
		ClientLimit: fairClients,
		PerClient:   per,
	})
	if err != nil {
		return 0, err
	}
	items := append(queued, reserved...)
	n := 0
	for i := range items {
		if !canStartLookupIO(w.now(), deadline) {
			break
		}
		w.submitItem(ctx, items[i], view, deadline)
		n++
	}
	return n, nil
}

func (w *Worker) submitItem(ctx context.Context, item sqlcdb.LookupItem, view settings.Public, deadline time.Time) {
	if _, err := w.store.Queries.MarkLookupJobProcessing(ctx, item.JobID); err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
		w.log.Error("lookup mark processing", "job_id", item.JobID, "err", err)
	}

	client, err := w.store.Queries.GetClientByID(ctx, item.ClientID)
	if err != nil {
		w.failItem(ctx, item, "client_not_found", "client not found", true)
		return
	}
	if client.Status != sqlcdb.ClientStatusActive {
		w.failItem(ctx, item, "client_suspended", "client is not active", true)
		return
	}
	if err := w.billing.AssertLookupAssignment(ctx, nil, item.ClientID, item.CheckType); err != nil {
		w.failItem(ctx, item, "tariff_not_configured", "tariff not configured", true)
		return
	}
	if w.provider == nil || !w.provider.Configured() {
		w.failItem(ctx, item, "provider_not_configured", "SMSC adapter is not configured", true)
		return
	}
	if w.limiter != nil {
		ok, retry, err := w.limiter.AllowRate(ctx, smscRateKey, view.ProviderRPS, burst(view.ProviderRPS))
		if err != nil && w.log != nil {
			w.log.Error("lookup smsc rps", "err", err)
		}
		if err == nil && !ok {
			_ = retry
			return
		}
	}

	in := smsc.SubmitInput{
		PhoneE164:      item.PhoneE164,
		IdempotencyKey: item.ID.String(),
		TenantID:       item.ClientID.String(),
		JobItemID:      item.ID.String(),
		CorrelationID:  item.ID.String(),
	}
	ioCtx, cancel := lookupIOContext(ctx, deadline)
	defer cancel()
	var result smsc.SubmitResult
	if item.CheckType == sqlcdb.LookupCheckTypePing {
		result, err = w.provider.SubmitPing(ioCtx, in)
	} else {
		result, err = w.provider.SubmitHLR(ioCtx, in)
	}
	if err != nil {
		if isRetryableLookupIO(err) {
			return
		}
		code := "submit_failed"
		msg := err.Error()
		if pe := smsc.AsError(err); pe != nil {
			if pe.ProviderErrorCode != nil {
				code = asStringCode(pe.ProviderErrorCode)
			} else {
				code = string(pe.Kind)
			}
			msg = pe.Message
		}
		w.failItem(ctx, item, code, msg, true)
		return
	}

	next, ok := MapLifecycleToItemStatus(result.Normalized.LifecycleStatus, sqlcdb.LookupItemStatusReserved)
	if !ok {
		next = sqlcdb.LookupItemStatusPending
	}
	now := w.now()
	pollAt := now.Add(time.Duration(view.LookupPollIntervalSec) * time.Second)
	if IsTerminalItem(next) {
		w.applyTerminalFromSubmit(ctx, item, result, next)
		return
	}
	n := result.Normalized
	attempts := int32(0)
	_, err = w.store.Queries.TransitionLookupItem(ctx, sqlcdb.TransitionLookupItemParams{
		ToStatus:          sqlcdb.LookupItemStatusPending,
		ProviderCode:      strPtr(result.ProviderCode),
		ProviderMessageID: strPtr(result.ProviderMessageID),
		ResultStatus:      strPtr(string(n.ResultStatus)),
		IsReachable:       n.IsReachable,
		Imsi:              strPtr(n.IMSI),
		Mcc:               strPtr(n.MCC),
		Mnc:               strPtr(n.MNC),
		OperatorName:      strPtr(n.OperatorName),
		CountryCode:       strPtr(n.CountryCode),
		Ported:            n.Ported,
		Roaming:           n.Roaming,
		NormalizedResult:  normalizedToJSON(n),
		NextPollAt:        &pollAt,
		PollAttempts:      &attempts,
		SentAt:            &now,
		ID:                item.ID,
		FromStatuses:      []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusReserved, sqlcdb.LookupItemStatusQueued},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
		w.log.Error("lookup submit update", "item_id", item.ID, "err", err)
	}
}

func (w *Worker) applyTerminalFromSubmit(ctx context.Context, item sqlcdb.LookupItem, result smsc.SubmitResult, next sqlcdb.LookupItemStatus) {
	n := result.Normalized
	now := w.now()
	updated, err := w.store.Queries.TransitionLookupItem(ctx, sqlcdb.TransitionLookupItemParams{
		ToStatus:          next,
		ProviderCode:      strPtr(result.ProviderCode),
		ProviderMessageID: strPtr(result.ProviderMessageID),
		ResultStatus:      strPtr(string(n.ResultStatus)),
		IsReachable:       n.IsReachable,
		Imsi:              strPtr(n.IMSI),
		Mcc:               strPtr(n.MCC),
		Mnc:               strPtr(n.MNC),
		OperatorName:      strPtr(n.OperatorName),
		CountryCode:       strPtr(n.CountryCode),
		Ported:            n.Ported,
		Roaming:           n.Roaming,
		NormalizedResult:  normalizedToJSON(n),
		SentAt:            &now,
		CompletedAt:       &now,
		ID:                item.ID,
		FromStatuses:      []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusReserved, sqlcdb.LookupItemStatusQueued},
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
			w.log.Error("lookup terminal from submit", "item_id", item.ID, "err", err)
		}
		return
	}
	w.onItemTerminal(ctx, updated, billing.LookupItemSettleAction(updated))
}

func (w *Worker) poll(ctx context.Context, deadline time.Time) (int, error) {
	view, err := w.runtime(ctx)
	if err != nil {
		return 0, err
	}
	per := int32(SubmitBatchSize / fairClients)
	if per < 1 {
		per = 1
	}
	items, err := w.store.Queries.ClaimPendingLookupItemsFair(ctx, sqlcdb.ClaimPendingLookupItemsFairParams{
		ClientLimit: fairClients,
		PerClient:   per,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range items {
		if !canStartLookupIO(w.now(), deadline) {
			w.releasePendingLease(ctx, items[i])
			continue
		}
		w.pollItem(ctx, items[i], view, deadline)
		n++
	}
	return n, nil
}

func (w *Worker) pollItem(ctx context.Context, item sqlcdb.LookupItem, view settings.Public, deadline time.Time) {
	if IsTerminalItem(item.Status) {
		_, _ = w.store.Queries.RefreshLookupJobCounters(ctx, item.JobID)
		return
	}
	if item.ProviderMessageID == nil || *item.ProviderMessageID == "" {
		w.failItem(ctx, item, "missing_provider_message_id", "Cannot poll without provider_message_id", true)
		return
	}
	start := item.SentAt
	if start == nil {
		start = &item.CreatedAt
	}
	age := w.now().Sub(*start)
	timeout := time.Duration(view.LookupCheckTimeoutSec) * time.Second
	if age >= timeout || item.PollAttempts >= PollMaxAttempts {
		w.failItem(ctx, item, "check_timeout", "Timed out waiting for provider final status", true)
		return
	}
	if w.provider == nil || !w.provider.Configured() {
		w.reschedulePoll(ctx, item, view)
		return
	}
	details := true
	ioCtx, cancel := lookupIOContext(ctx, deadline)
	defer cancel()
	status, err := w.provider.FetchStatus(ioCtx, smsc.FetchStatusInput{
		ProviderMessageID: *item.ProviderMessageID,
		PhoneE164:         item.PhoneE164,
		CheckType:         smsc.NormalizeCheckType(smsc.CheckType(item.CheckType)),
		TenantID:          item.ClientID.String(),
		JobItemID:         item.ID.String(),
		CorrelationID:     item.ID.String(),
		IncludeDetails:    &details,
	})
	if err != nil {
		if isRetryableLookupIO(err) || smsc.AsError(err) == nil {
			if item.PollAttempts+1 < PollMaxAttempts {
				w.reschedulePoll(ctx, item, view)
				return
			}
		}
		code := "poll_failed"
		msg := err.Error()
		if pe := smsc.AsError(err); pe != nil {
			if pe.ProviderErrorCode != nil {
				code = asStringCode(pe.ProviderErrorCode)
			} else {
				code = string(pe.Kind)
			}
			msg = pe.Message
		}
		w.failItem(ctx, item, code, msg, true)
		return
	}
	applied, err := w.ApplyProviderUpdate(ctx, ApplyInput{
		JobItemID:         item.ID,
		ClientID:          item.ClientID,
		ProviderMessageID: *item.ProviderMessageID,
		Normalized:        status.Normalized,
		Source:            "poll",
		Deadline:          deadline,
	})
	if err != nil && w.log != nil {
		w.log.Error("lookup apply poll", "item_id", item.ID, "err", err)
	}
	if applied.BecameTerminal {
		return
	}
	w.reschedulePoll(ctx, item, view)
}

func (w *Worker) reschedulePoll(ctx context.Context, item sqlcdb.LookupItem, view settings.Public) {
	attempt := item.PollAttempts + 1
	next := w.now().Add(backoff(view.LookupPollIntervalSec, attempt))
	_, err := w.store.Queries.TransitionLookupItem(ctx, sqlcdb.TransitionLookupItemParams{
		ToStatus:     sqlcdb.LookupItemStatusPending,
		NextPollAt:   &next,
		PollAttempts: &attempt,
		ID:           item.ID,
		FromStatuses: []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusPending},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
		w.log.Error("lookup reschedule poll", "item_id", item.ID, "err", err)
	}
}

type ApplyInput struct {
	JobItemID         uuid.UUID
	ClientID          uuid.UUID
	ProviderMessageID string
	Normalized        smsc.NormalizedResult
	Source            string
	Deadline          time.Time
	SkipEnrich        bool
}

type ApplyResult struct {
	Applied        bool
	Duplicate      bool
	Item           sqlcdb.LookupItem
	BecameTerminal bool
	Reason         string
}

func (w *Worker) ApplyProviderUpdate(ctx context.Context, in ApplyInput) (ApplyResult, error) {
	var item sqlcdb.LookupItem
	var err error
	if in.JobItemID != uuid.Nil {
		item, err = w.store.Queries.GetLookupItem(ctx, in.JobItemID)
	} else if in.ProviderMessageID != "" {
		item, err = w.store.Queries.GetLookupItemByProviderMessage(ctx, sqlcdb.GetLookupItemByProviderMessageParams{
			ProviderCode:      smsc.ProviderCode,
			ProviderMessageID: &in.ProviderMessageID,
		})
	} else {
		return ApplyResult{}, wrap(ErrNotFound, "not_found", "JobItem not found for provider update")
	}
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ApplyResult{}, wrap(ErrNotFound, "not_found", "JobItem not found for provider update")
		}
		return ApplyResult{}, err
	}
	if in.ClientID != uuid.Nil && item.ClientID != in.ClientID {
		return ApplyResult{}, wrap(ErrNotFound, "not_found", "JobItem not found for client")
	}

	if IsTerminalItem(item.Status) {
		if !in.SkipEnrich && item.Status == sqlcdb.LookupItemStatusCompleted && item.CheckType == sqlcdb.LookupCheckTypeHlr {
			enriched, _ := w.enrichHLR(ctx, item, in.Normalized, in.Deadline)
			if hlrFieldsImproved(enriched, item) {
				patched, err := w.patchItem(ctx, item, enriched, sqlcdb.LookupItemStatusCompleted, []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusCompleted}, false)
				if err == nil {
					return ApplyResult{Applied: true, Item: patched}, nil
				}
			}
		}
		return ApplyResult{Duplicate: true, Item: item}, nil
	}

	normalized := mergeNormalizedWithItem(in.Normalized, item)
	next, ok := MapLifecycleToItemStatus(normalized.LifecycleStatus, item.Status)
	if !ok {
		return ApplyResult{Item: item, Reason: "lifecycle_unmapped"}, nil
	}
	if !in.SkipEnrich && (next == sqlcdb.LookupItemStatusCompleted || next == sqlcdb.LookupItemStatusFailed) {
		normalized, _ = w.enrichHLR(ctx, item, normalized, in.Deadline)
	}

	from := []sqlcdb.LookupItemStatus{item.Status}
	if next == sqlcdb.LookupItemStatusPending {
		from = []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusPending, sqlcdb.LookupItemStatusReserved}
		patched, err := w.patchItem(ctx, item, normalized, sqlcdb.LookupItemStatusPending, from, false)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ApplyResult{Item: item, Reason: "race"}, nil
			}
			return ApplyResult{}, err
		}
		return ApplyResult{Applied: true, Item: patched}, nil
	}

	from = []sqlcdb.LookupItemStatus{
		sqlcdb.LookupItemStatusQueued,
		sqlcdb.LookupItemStatusReserved,
		sqlcdb.LookupItemStatusPending,
	}
	patched, err := w.patchItem(ctx, item, normalized, next, from, true)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			fresh, _ := w.store.Queries.GetLookupItem(ctx, item.ID)
			return ApplyResult{Duplicate: IsTerminalItem(fresh.Status), Item: fresh}, nil
		}
		return ApplyResult{}, err
	}
	w.onItemTerminal(ctx, patched, billing.LookupItemSettleAction(patched))
	return ApplyResult{Applied: true, Item: patched, BecameTerminal: true}, nil
}

func (w *Worker) enrichHLR(ctx context.Context, item sqlcdb.LookupItem, normalized smsc.NormalizedResult, deadline time.Time) (smsc.NormalizedResult, bool) {
	merged := mergeNormalizedWithItem(normalized, item)
	if !needsHLREnrich(merged, item.CheckType) {
		return merged, false
	}
	if !canStartLookupIO(w.now(), deadline) {
		return merged, false
	}
	providerMessageID := preferString(merged.ProviderMessageID, deref(item.ProviderMessageID))
	if providerMessageID == "" || w.provider == nil || !w.provider.Configured() {
		return merged, false
	}
	details := true
	ioCtx, cancel := lookupIOContext(ctx, deadline)
	defer cancel()
	status, err := w.provider.FetchStatus(ioCtx, smsc.FetchStatusInput{
		CheckType:         smsc.CheckHLR,
		PhoneE164:         item.PhoneE164,
		ProviderMessageID: providerMessageID,
		TenantID:          item.ClientID.String(),
		JobItemID:         item.ID.String(),
		IncludeDetails:    &details,
	})
	if err != nil {
		if w.log != nil {
			w.log.Warn("lookup hlr enrich failed", "item_id", item.ID, "err", err)
		}
		return merged, true
	}
	return mergeEnrich(merged, status.Normalized), true
}

// bumpEnrichAttempt consumes an enrich attempt unless the Tick budget expired
// before SMSC was called. A real FetchStatus (success or error) always counts.
// Claimed items that cannot be enriched (no provider id) also count, so they
// are not reclaimed forever.
func bumpEnrichAttempt(invokedProvider, budgetExpired bool) bool {
	if budgetExpired && !invokedProvider {
		return false
	}
	return true
}

func (w *Worker) patchItem(ctx context.Context, item sqlcdb.LookupItem, n smsc.NormalizedResult, to sqlcdb.LookupItemStatus, from []sqlcdb.LookupItemStatus, terminal bool) (sqlcdb.LookupItem, error) {
	now := w.now()
	arg := sqlcdb.TransitionLookupItemParams{
		ToStatus:          to,
		ProviderMessageID: strPtr(preferString(n.ProviderMessageID, deref(item.ProviderMessageID))),
		ResultStatus:      strPtr(string(n.ResultStatus)),
		IsReachable:       n.IsReachable,
		Imsi:              strPtr(n.IMSI),
		Mcc:               strPtr(n.MCC),
		Mnc:               strPtr(n.MNC),
		OperatorName:      strPtr(n.OperatorName),
		CountryCode:       strPtr(n.CountryCode),
		Ported:            n.Ported,
		Roaming:           n.Roaming,
		NormalizedResult:  normalizedToJSON(n),
		ErrorCode:         strPtr(n.ProviderErrorCode),
		ErrorMessage:      strPtr(n.ProviderErrorMessage),
		ID:                item.ID,
		FromStatuses:      from,
	}
	if terminal {
		arg.CompletedAt = &now
	} else if to == sqlcdb.LookupItemStatusPending {
		if item.SentAt == nil {
			arg.SentAt = &now
		}
		if item.NextPollAt == nil {
			pollAt := now.Add(30 * time.Second)
			arg.NextPollAt = &pollAt
		}
	}
	return w.store.Queries.TransitionLookupItem(ctx, arg)
}

func (w *Worker) failItem(ctx context.Context, item sqlcdb.LookupItem, code, message string, release bool) {
	now := w.now()
	updated, err := w.store.Queries.TransitionLookupItem(ctx, sqlcdb.TransitionLookupItemParams{
		ToStatus:     sqlcdb.LookupItemStatusFailed,
		ErrorCode:    &code,
		ErrorMessage: &message,
		CompletedAt:  &now,
		ID:           item.ID,
		FromStatuses: []sqlcdb.LookupItemStatus{
			sqlcdb.LookupItemStatusQueued,
			sqlcdb.LookupItemStatusReserved,
			sqlcdb.LookupItemStatusPending,
		},
	})
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
			w.log.Error("lookup fail item", "item_id", item.ID, "code", code, "err", err)
		}
		return
	}
	settle := "release"
	if !release {
		settle = "capture"
	}
	w.onItemTerminal(ctx, updated, settle)
}

func (w *Worker) onItemTerminal(ctx context.Context, item sqlcdb.LookupItem, action string) {
	if !IsTerminalItem(item.Status) {
		return
	}
	if action == "" {
		action = billing.LookupItemSettleAction(item)
	}
	if w.billing != nil {
		var err error
		switch action {
		case "release":
			err = w.billing.ReleaseForLookupItem(ctx, item.ID)
		case "capture":
			err = w.billing.CaptureForLookupItem(ctx, item.ID)
		default:
			if w.log != nil {
				w.log.Error("lookup item settle skipped", "item_id", item.ID, "action", action)
			}
		}
		if err != nil && w.log != nil {
			w.log.Error("lookup item settle", "item_id", item.ID, "action", action, "err", err)
		}
	}
	if _, err := w.store.Queries.RefreshLookupJobCounters(ctx, item.JobID); err != nil && w.log != nil {
		w.log.Error("lookup refresh counters", "job_id", item.JobID, "err", err)
	}
	if w.hooks != nil {
		if _, err := w.hooks.EnqueueItem(ctx, item); err != nil && w.log != nil {
			w.log.Error("lookup webhook item", "item_id", item.ID, "err", err)
		}
	}
}

func (w *Worker) finalizeReady(ctx context.Context) (int, error) {
	jobs, err := w.store.Queries.ListJobsNeedingFinalize(ctx, 20)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range jobs {
		if _, err := w.FinalizeJob(ctx, jobs[i].ID); err != nil && w.log != nil {
			w.log.Error("lookup finalize job", "job_id", jobs[i].ID, "err", err)
			continue
		}
		n++
	}
	return n, nil
}

func (w *Worker) FinalizeJob(ctx context.Context, jobID uuid.UUID) (sqlcdb.LookupJob, error) {
	job, err := w.store.Queries.RefreshLookupJobCounters(ctx, jobID)
	if err != nil {
		return sqlcdb.LookupJob{}, err
	}
	if IsTerminalJob(job.Status) {
		w.releaseRemainder(ctx, job)
		return job, nil
	}
	progress := ComputeProgress(job.ItemCount, job.SuccessCount, job.FailureCount)
	if job.ItemCount == 0 || progress.Pending > 0 {
		return job, nil
	}
	terminal := DeriveJobTerminalStatus(progress.Total, progress.Success, progress.Failed)
	finalized, err := w.store.Queries.FinalizeLookupJob(ctx, sqlcdb.FinalizeLookupJobParams{
		Status: terminal,
		ID:     job.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return w.store.Queries.GetLookupJob(ctx, job.ID)
		}
		return sqlcdb.LookupJob{}, err
	}
	w.releaseRemainder(ctx, finalized)
	if w.hooks != nil {
		if _, err := w.hooks.EnqueueJob(ctx, finalized); err != nil && w.log != nil {
			w.log.Error("lookup webhook job", "job_id", finalized.ID, "err", err)
		}
	}
	return finalized, nil
}

func (w *Worker) deliverWebhooks(ctx context.Context) (int, error) {
	if w.hooks == nil {
		return 0, nil
	}
	return w.hooks.DeliverDue(ctx, 10)
}

func (w *Worker) releaseRemainder(ctx context.Context, job sqlcdb.LookupJob) {
	if w.billing == nil || !IsTerminalJob(job.Status) {
		return
	}
	if err := w.billing.ReleaseLookupJobRemainder(ctx, job.ID); err != nil && w.log != nil {
		w.log.Error("lookup release remainder", "job_id", job.ID, "err", err)
	}
}

func (w *Worker) reconcile(ctx context.Context) error {
	if n, err := w.store.Queries.ReopenUnappliedLookupCallbacks(ctx, w.now().Add(-24*time.Hour)); err != nil {
		if w.log != nil {
			w.log.Error("lookup callback reopen", "err", err)
		}
	} else if n > 0 && w.log != nil {
		w.log.Info("lookup callbacks reopened", "n", n)
	}
	view, err := w.runtime(ctx)
	if err != nil {
		return err
	}
	older := w.now().Add(-time.Duration(view.LookupPollIntervalSec) * time.Second)
	stalePending, err := w.store.Queries.ListStalePendingLookupItems(ctx, sqlcdb.ListStalePendingLookupItemsParams{
		OlderThan: older,
		PageLimit: 100,
	})
	if err != nil {
		return err
	}
	now := w.now()
	for i := range stalePending {
		item := stalePending[i]
		_, _ = w.store.Queries.TransitionLookupItem(ctx, sqlcdb.TransitionLookupItemParams{
			ToStatus:     sqlcdb.LookupItemStatusPending,
			NextPollAt:   &now,
			ID:           item.ID,
			FromStatuses: []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusPending},
		})
	}

	staleReserved, err := w.store.Queries.ListStaleReservedLookupItems(ctx, sqlcdb.ListStaleReservedLookupItemsParams{
		OlderThan: older,
		PageLimit: 100,
	})
	if err != nil {
		return err
	}
	timeout := time.Duration(view.LookupCheckTimeoutSec) * time.Second
	for i := range staleReserved {
		item := staleReserved[i]
		if w.now().Sub(item.CreatedAt) >= timeout {
			w.failItem(ctx, item, "reserved_stale_timeout", "RESERVED item exceeded check timeout without submit progress", true)
		}
	}

	if _, err := w.finalizeReady(ctx); err != nil {
		return err
	}

	shells, err := w.store.Queries.ListEmptyCsvLookupShells(ctx, sqlcdb.ListEmptyCsvLookupShellsParams{
		OlderThan: w.now().Add(-csvShellAge),
		PageLimit: 20,
	})
	if err != nil {
		return err
	}
	for i := range shells {
		if w.svc != nil {
			_, _ = w.svc.FailJob(ctx, shells[i].ID, "csv_parse_abandoned", "CSV parse heal attempts exhausted")
			continue
		}
		_, _ = w.store.Queries.FinalizeLookupJob(ctx, sqlcdb.FinalizeLookupJobParams{
			Status:       sqlcdb.LookupJobStatusFailed,
			ErrorCode:    strPtr("csv_parse_abandoned"),
			ErrorMessage: strPtr("CSV parse heal attempts exhausted"),
			ID:           shells[i].ID,
		})
	}

	if _, err := w.store.Queries.HealStaleConsumingLookupCSVPreviews(ctx, w.now().Add(-csvConsumingAge)); err != nil && w.log != nil {
		w.log.Error("lookup csv consuming heal", "err", err)
	}

	if w.billing != nil {
		if _, err := w.billing.ReapOpenLookupHolds(ctx, 50); err != nil && w.log != nil {
			w.log.Error("lookup hold reaper", "err", err)
		}
	}
	return nil
}

func (w *Worker) parsePendingCSV(ctx context.Context) (int, error) {
	if w.svc == nil {
		return 0, nil
	}
	rows, err := w.store.Queries.ClaimLookupCSVPreviewsPendingParse(ctx, 5)
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range rows {
		if err := w.svc.MaterializeCSVJob(ctx, rows[i]); err != nil {
			if w.log != nil {
				w.log.Error("lookup materialize csv", "preview_id", rows[i].ID, "err", err)
			}
			w.svc.rollbackPreview(ctx, rows[i].ID)
			continue
		}
		n++
	}
	return n, nil
}

func (w *Worker) enrichTerminalHLR(ctx context.Context, deadline time.Time) (int, error) {
	if w.provider == nil || !w.provider.Configured() {
		return 0, nil
	}
	items, err := w.store.Queries.ClaimLookupItemsNeedingHLREnrich(ctx, sqlcdb.ClaimLookupItemsNeedingHLREnrichParams{
		MaxAttempts: EnrichMaxAttempts,
		PageLimit:   8,
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for i := range items {
		if !canStartLookupIO(w.now(), deadline) {
			break
		}
		item := items[i]
		enriched, invoked := w.enrichHLR(ctx, item, mergeNormalizedWithItem(smsc.NormalizedResult{}, item), deadline)
		if hlrFieldsImproved(enriched, item) {
			from := []sqlcdb.LookupItemStatus{item.Status}
			if _, err := w.patchItem(ctx, item, enriched, item.Status, from, false); err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
				w.log.Error("lookup hlr enrich patch", "item_id", item.ID, "err", err)
			}
		}
		if bumpEnrichAttempt(invoked, !canStartLookupIO(w.now(), deadline)) {
			if err := w.store.Queries.BumpLookupItemEnrichAttempt(ctx, item.ID); err != nil && w.log != nil {
				w.log.Error("lookup hlr enrich bump", "item_id", item.ID, "err", err)
			}
		}
		n++
	}
	return n, nil
}

func (w *Worker) releasePendingLease(ctx context.Context, item sqlcdb.LookupItem) {
	now := w.now()
	_, err := w.store.Queries.TransitionLookupItem(ctx, sqlcdb.TransitionLookupItemParams{
		ToStatus:     sqlcdb.LookupItemStatusPending,
		NextPollAt:   &now,
		ID:           item.ID,
		FromStatuses: []sqlcdb.LookupItemStatus{sqlcdb.LookupItemStatusPending},
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
		w.log.Error("lookup return pending lease", "item_id", item.ID, "err", err)
	}
}

func isRetryableLookupIO(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if pe := smsc.AsError(err); pe != nil {
		return pe.Retryable
	}
	return false
}

func canStartLookupIO(now, deadline time.Time) bool {
	if deadline.IsZero() {
		return true
	}
	return deadline.Sub(now) >= minHTTPBudget
}

func lookupIOContext(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if deadline.IsZero() {
		return ctx, func() {}
	}
	// Tick deadline only decides whether to start another call. A started
	// SMSC request is capped so one hang cannot stall CSV/submit/poll in
	// this Tick (adapter timeout is 15s). Canceled calls are retryable.
	return context.WithTimeout(ctx, smscCallTimeout)
}

func backoff(baseSec int32, attempt int32) time.Duration {
	if baseSec < 1 {
		baseSec = 30
	}
	exp := attempt - 1
	if exp < 0 {
		exp = 0
	}
	if exp > 6 {
		exp = 6
	}
	return time.Duration(float64(baseSec)*math.Pow(2, float64(exp))) * time.Second
}

func burst(rate float64) float64 {
	if rate < 1 {
		return 1
	}
	return rate
}

func asStringCode(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

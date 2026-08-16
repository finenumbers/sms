package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/messaging"
	"finenumbers/sms/internal/metrics"
	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/ratelimit"
	"finenumbers/sms/internal/runexis"
	"finenumbers/sms/internal/settings"
)

const (
	needStat          = "uncertain:need_stat"
	needRetry         = "uncertain:need_retry"
	maxUncertainSends = 2
	staleProcessing   = 2 * time.Minute
	uncertainPause    = 15 * time.Second
	retryAfterStat    = 5 * time.Second
	claimBatch        = 8
	bodyTruncateRunes = 2048
	reconcileAge      = 2 * time.Minute
	inboxWindow       = 30 * time.Minute
)

type SettingsView interface {
	Get(ctx context.Context) (settings.Public, error)
}

type Worker struct {
	store    *db.Store
	rx       *runexis.Client
	limiter  *ratelimit.Limiter
	settings SettingsView
	billing  *billing.Service
	log      *slog.Logger
	ops      *ops.Logger
	workerID string
	now      func() time.Time
	lastRec  time.Time
	statUsed map[string]struct{}
}

func NewWorker(store *db.Store, rx *runexis.Client, limiter *ratelimit.Limiter, settings SettingsView, bill *billing.Service, log *slog.Logger, opsLog *ops.Logger) *Worker {
	host, _ := os.Hostname()
	id := fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString()[:8])
	if log == nil {
		log = slog.Default()
	}
	return &Worker{
		store:    store,
		rx:       rx,
		limiter:  limiter,
		settings: settings,
		billing:  bill,
		log:      log,
		ops:      opsLog,
		workerID: id,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (w *Worker) Tick(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	w.statUsed = map[string]struct{}{}
	stale := w.now().Add(-staleProcessing)
	if _, err := w.store.Queries.ReclaimStaleSendJobs(ctx, &stale); err != nil {
		return 0, err
	}
	jobs, err := w.store.Queries.ClaimSendJobsFair(ctx, sqlcdb.ClaimSendJobsFairParams{
		WorkerID:   &w.workerID,
		BatchLimit: claimBatch,
	})
	if err != nil {
		return 0, err
	}
	for i := range jobs {
		w.process(ctx, jobs[i])
	}
	if w.lastRec.IsZero() || w.now().Sub(w.lastRec) >= 15*time.Second {
		recCtx := ops.ContextWith(ctx, ops.Fields{RequestID: "reconcile"})
		if err := w.reconcile(recCtx); err != nil && w.log != nil {
			w.log.Error("reconcile", "err", err)
		}
		w.lastRec = w.now()
	}
	if w.billing != nil {
		if _, err := w.billing.ReapOpenHolds(ctx, 50); err != nil && w.log != nil {
			w.log.Error("billing reaper", "err", err)
		}
	}
	return len(jobs), nil
}

func (w *Worker) process(ctx context.Context, job sqlcdb.SendJob) {
	ctx = ops.ContextWith(ctx, ops.Fields{
		RequestID:    "job:" + job.ID.String(),
		ClientID:     job.ClientID,
		ResourceType: "sms_message",
		ResourceID:   &job.SmsMessageID,
	})
	msg, err := w.store.Queries.GetSmsMessageByID(ctx, job.SmsMessageID)
	if err != nil {
		w.park(ctx, job, sqlcdb.SendJobStatusDead, job.Attempt, deref(job.LastError), 0)
		return
	}
	if job.ClientID != nil {
		cl, err := w.store.Queries.GetClientByID(ctx, *job.ClientID)
		if err != nil || cl.Status == sqlcdb.ClientStatusDeleted {
			w.park(ctx, job, sqlcdb.SendJobStatusDead, job.Attempt, "client deleted", 0)
			return
		}
	}
	if needStatistic(job) {
		w.handleUncertain(ctx, job, msg)
		return
	}
	w.send(ctx, job, msg)
}

func (w *Worker) handleUncertain(ctx context.Context, job sqlcdb.SendJob, msg sqlcdb.SmsMessage) {
	row, ok, err := w.lookupStatistic(ctx, msg)
	if err != nil {
		if errors.Is(err, runexis.ErrNotConfigured) {
			w.park(ctx, job, sqlcdb.SendJobStatusUncertain, job.Attempt, needStat, 30*time.Second)
			return
		}
		w.park(ctx, job, sqlcdb.SendJobStatusUncertain, job.Attempt, needStat, uncertainPause)
		return
	}
	if ok {
		w.acceptFromStat(ctx, job, msg, row)
		return
	}
	if !allowUncertainRetry(job.Attempt, w.latestAttemptKind(ctx, job.ID)) {
		w.failDead(ctx, job, msg, job.Attempt, "uncertain: not found in statistic after retry")
		return
	}
	if deref(job.LastError) == needRetry {
		w.send(ctx, job, msg)
		return
	}
	w.park(ctx, job, sqlcdb.SendJobStatusUncertain, job.Attempt, needRetry, retryAfterStat)
}

func (w *Worker) latestAttemptKind(ctx context.Context, jobID uuid.UUID) sqlcdb.SendAttemptKind {
	if w == nil || w.store == nil {
		return ""
	}
	att, err := w.store.Queries.GetLatestSendAttempt(ctx, jobID)
	if err != nil || !att.ErrorKind.Valid {
		return ""
	}
	return att.ErrorKind.SendAttemptKind
}

// allowUncertainRetry is true only for timeout/network (maybe the SMS already left).
// An explicit HTTP 5xx means the provider answered; a second POST would duplicate.
func allowUncertainRetry(attempt int32, kind sqlcdb.SendAttemptKind) bool {
	if attempt >= maxUncertainSends {
		return false
	}
	if kind == sqlcdb.SendAttemptKind5xx {
		return false
	}
	return true
}

func (w *Worker) send(ctx context.Context, job sqlcdb.SendJob, msg sqlcdb.SmsMessage) {
	owned, err := w.touchLock(ctx, job)
	if err != nil || !owned {
		return
	}
	if !w.allowSend(ctx, job) {
		return
	}
	if w.rx == nil {
		w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, "runexis adapter unavailable", 30*time.Second)
		return
	}
	start := w.now()
	res, err := w.rx.Send(ctx, runexis.SendInput{From: msg.FromMsisdn, To: msg.ToMsisdn, Text: msg.Text})
	latency := w.now().Sub(start)
	attempt := job.Attempt + 1
	kind, status := classify(err)
	w.recordAttempt(ctx, job.ID, attempt, msg, status, res, err, latency, kind)

	if err == nil {
		w.accept(ctx, job, msg, res.ProviderSMSID, attempt)
		return
	}
	if errors.Is(err, runexis.ErrNotConfigured) {
		w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, err.Error(), 30*time.Second)
		return
	}
	switch kind {
	case sqlcdb.SendAttemptKindRejected4xx:
		w.failDead(ctx, job, msg, attempt, err.Error())
	case sqlcdb.SendAttemptKindRateLimited:
		if w.limiter != nil {
			_ = w.limiter.Drain(ctx, "rl:provider")
		}
		w.park(ctx, job, sqlcdb.SendJobStatusRetry, attempt, err.Error(), backoff429(attempt))
	default:
		w.park(ctx, job, sqlcdb.SendJobStatusUncertain, attempt, needStat, uncertainPause)
	}
}

func (w *Worker) allowSend(ctx context.Context, job sqlcdb.SendJob) bool {
	view, err := w.settings.Get(ctx)
	if err != nil {
		w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, "settings unavailable", 5*time.Second)
		return false
	}
	if w.limiter == nil {
		return true
	}
	if job.ClientID != nil {
		ok, retry, err := w.limiter.AllowRate(ctx, "rl:worker:client:"+job.ClientID.String(), view.ClientRPSDefault, burst(view.ClientRPSDefault))
		if err != nil {
			w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, "rate limiter unavailable", 5*time.Second)
			return false
		}
		if !ok {
			w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, "client_rps", retry)
			return false
		}
	}
	ok, retry, err := w.limiter.AllowRate(ctx, "rl:provider", view.ProviderRPS, burst(view.ProviderRPS))
	if err != nil {
		w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, "rate limiter unavailable", 5*time.Second)
		return false
	}
	if !ok {
		w.park(ctx, job, sqlcdb.SendJobStatusRetry, job.Attempt, "provider_rps", retry)
		return false
	}
	return true
}

func (w *Worker) accept(ctx context.Context, job sqlcdb.SendJob, msg sqlcdb.SmsMessage, providerID string, attempt int32) {
	var pid *string
	if providerID != "" {
		pid = &providerID
	}
	if err := markAccepted(ctx, w.store.Queries, msg.ID, pid); err != nil {
		if w.log != nil {
			w.log.Error("accept message", "id", msg.ID, "err", err)
		}
		w.park(ctx, job, sqlcdb.SendJobStatusUncertain, attempt, needStat, uncertainPause)
		return
	}
	w.capture(ctx, msg.ID)
	w.noteSMSID(providerID)
	w.complete(ctx, job)
}

func (w *Worker) acceptFromStat(ctx context.Context, job sqlcdb.SendJob, msg sqlcdb.SmsMessage, row runexis.StatisticRow) {
	w.noteSMSID(row.SMSID)
	if err := applyStatistic(ctx, w.store.Queries, msg.ID, row); err != nil {
		if w.log != nil {
			w.log.Error("apply statistic", "id", msg.ID, "err", err)
		}
		w.park(ctx, job, sqlcdb.SendJobStatusUncertain, job.Attempt, needStat, uncertainPause)
		return
	}
	w.capture(ctx, msg.ID)
	w.complete(ctx, job)
}

func (w *Worker) failDead(ctx context.Context, job sqlcdb.SendJob, msg sqlcdb.SmsMessage, attempt int32, reason string) {
	if !w.park(ctx, job, sqlcdb.SendJobStatusDead, attempt, reason, 0) {
		return
	}
	st := providerStatusForJob(ctx, w.store.Queries, job.ID, reason)
	if _, err := w.store.Queries.UpdateSmsMessageFailed(ctx, sqlcdb.UpdateSmsMessageFailedParams{
		ProviderStatus: &st,
		ID:             msg.ID,
	}); err != nil && w.log != nil {
		w.log.Error("fail message", "id", msg.ID, "err", err)
	}
	w.release(ctx, msg.ID)
	if w.log != nil {
		w.log.Error("send job dead", "job_id", job.ID, "message_id", msg.ID, "reason", st)
	}
}

func (w *Worker) touchLock(ctx context.Context, job sqlcdb.SendJob) (bool, error) {
	wid := w.workerID
	_, err := w.store.Queries.TouchSendJobLock(ctx, sqlcdb.TouchSendJobLockParams{
		ID:       job.ID,
		WorkerID: &wid,
	})
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if w.log != nil {
		w.log.Error("touch send job lock", "id", job.ID, "err", err)
	}
	return false, err
}

func (w *Worker) complete(ctx context.Context, job sqlcdb.SendJob) {
	wid := w.workerID
	n, err := w.store.Queries.CompleteSendJob(ctx, sqlcdb.CompleteSendJobParams{
		ID:       job.ID,
		WorkerID: &wid,
	})
	if err != nil && w.log != nil {
		w.log.Error("complete send job", "id", job.ID, "err", err)
		return
	}
	if n == 0 && w.log != nil {
		w.log.Info("complete send job skipped", "id", job.ID)
	}
}

func (w *Worker) park(ctx context.Context, job sqlcdb.SendJob, status sqlcdb.SendJobStatus, attempt int32, lastErr string, delay time.Duration) bool {
	var errp *string
	if lastErr != "" {
		errp = &lastErr
	}
	at := w.now()
	if delay > 0 {
		at = at.Add(delay)
	}
	wid := w.workerID
	n, err := w.store.Queries.ParkSendJob(ctx, sqlcdb.ParkSendJobParams{
		Status:      status,
		Attempt:     attempt,
		AvailableAt: at,
		LastError:   errp,
		ID:          job.ID,
		WorkerID:    &wid,
	})
	if err != nil {
		if w.log != nil {
			w.log.Error("park send job", "id", job.ID, "err", err)
		}
		return false
	}
	if n == 0 {
		return false
	}
	switch status {
	case sqlcdb.SendJobStatusRetry, sqlcdb.SendJobStatusUncertain, sqlcdb.SendJobStatusDead:
		level := ops.LevelWarn
		if status == sqlcdb.SendJobStatusDead {
			level = ops.LevelError
		}
		if w.ops != nil {
			w.ops.Write(ctx, ops.Event{
				Category:     ops.CategoryQueue,
				Level:        level,
				Action:       "send_job." + string(status),
				ResourceType: "send_job",
				ResourceID:   &job.ID,
				ClientID:     job.ClientID,
				Summary:      lastErr,
				Error:        lastErr,
				Detail: map[string]any{
					"status":         status,
					"attempt":        attempt,
					"sms_message_id": job.SmsMessageID,
				},
			})
		}
	}
	return true
}

func (w *Worker) recordAttempt(ctx context.Context, jobID uuid.UUID, attempt int32, msg sqlcdb.SmsMessage, httpStatus int, res runexis.SendResult, sendErr error, latency time.Duration, kind sqlcdb.SendAttemptKind) {
	meta, _ := json.Marshal(map[string]string{"from": msg.FromMsisdn, "to": msg.ToMsisdn})
	var body *string
	if len(res.Raw) > 0 {
		s := truncateRunes(string(ops.RedactJSON(res.Raw)), bodyTruncateRunes)
		body = &s
	} else if sendErr != nil {
		s := truncateRunes(sendErr.Error(), bodyTruncateRunes)
		body = &s
	}
	var status *int32
	if httpStatus > 0 {
		v := int32(httpStatus)
		status = &v
	}
	lat := int32(latency.Milliseconds())
	_ = w.store.Queries.InsertSendAttempt(ctx, sqlcdb.InsertSendAttemptParams{
		SendJobID:    jobID,
		Attempt:      attempt,
		RequestMeta:  meta,
		HttpStatus:   status,
		ResponseBody: body,
		LatencyMs:    &lat,
		ErrorKind:    sqlcdb.NullSendAttemptKind{SendAttemptKind: kind, Valid: true},
	})
}

func (w *Worker) lookupStatistic(ctx context.Context, msg sqlcdb.SmsMessage) (runexis.StatisticRow, bool, error) {
	if w.rx == nil {
		return runexis.StatisticRow{}, false, runexis.ErrNotConfigured
	}
	incoming := false
	from := msg.CreatedAt.Add(-2 * time.Minute)
	to := w.now().Add(time.Minute)
	page, err := w.rx.Statistic(ctx, runexis.StatisticQuery{
		From:            from,
		To:              to,
		SenderNumbers:   []string{msg.FromMsisdn},
		ReceiverNumbers: []string{msg.ToMsisdn},
		Incoming:        &incoming,
		Page:            1,
		Limit:           50,
	})
	if err != nil {
		return runexis.StatisticRow{}, false, err
	}
	used := w.statUsed
	if used == nil {
		used = map[string]struct{}{}
	}
	row, ok := messaging.MatchStatistic(msg.FromMsisdn, msg.ToMsisdn, msg.Text, page.Items, used)
	return row, ok, nil
}

func (w *Worker) reconcile(ctx context.Context) error {
	before := w.now().Add(-reconcileAge)
	msgs, err := w.store.Queries.ListStaleOutbound(ctx, sqlcdb.ListStaleOutboundParams{
		Before:    before,
		PageLimit: 20,
	})
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		row, ok, err := w.lookupStatistic(ctx, msg)
		if err != nil || !ok {
			continue
		}
		w.noteSMSID(row.SMSID)
		_ = applyStatistic(ctx, w.store.Queries, msg.ID, row)
		w.capture(ctx, msg.ID)
	}
	return w.backfillInbox(ctx)
}

func (w *Worker) backfillInbox(ctx context.Context) error {
	if w.rx == nil {
		return nil
	}
	incoming := true
	page, err := w.rx.Statistic(ctx, runexis.StatisticQuery{
		From:     w.now().Add(-inboxWindow),
		To:       w.now().Add(time.Minute),
		Incoming: &incoming,
		Page:     1,
		Limit:    50,
	})
	if err != nil {
		return err
	}
	for _, row := range page.Items {
		if !row.Incoming || row.SMSID == "" {
			continue
		}
		to := msisdn.Canonical(row.ReceiverNumber)
		from := msisdn.Canonical(row.SenderNumber)
		if to == "" || from == "" {
			continue
		}
		var clientID *uuid.UUID
		if asg, err := w.store.Queries.GetOpenAssignmentByMSISDN(ctx, to); err == nil {
			clientID = &asg.ClientID
		}
		text := row.Message
		var pdu *int32
		if row.PDU > 0 {
			v := int32(row.PDU)
			pdu = &v
		}
		sid := row.SMSID
		_, err := w.store.Queries.InsertInboundMessage(ctx, sqlcdb.InsertInboundMessageParams{
			ClientID:      clientID,
			FromMsisdn:    from,
			ToMsisdn:      to,
			Text:          text,
			ProviderSmsID: &sid,
			PduCount:      pdu,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
			w.log.Error("inbox backfill", "err", err)
		}
	}
	return nil
}

func applyStatistic(ctx context.Context, q *sqlcdb.Queries, id uuid.UUID, row runexis.StatisticRow) error {
	var pid *string
	if row.SMSID != "" {
		pid = &row.SMSID
	}
	var pdu *int32
	if row.PDU > 0 {
		v := int32(row.PDU)
		pdu = &v
	}
	_, err := q.UpdateSmsMessageFromStatistic(ctx, sqlcdb.UpdateSmsMessageFromStatisticParams{
		ProviderSmsID: pid,
		PduCount:      pdu,
		Delivered:     row.Delivered,
		Sent:          row.Sent,
		ID:            id,
	})
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		if msg, getErr := q.GetSmsMessageByID(ctx, id); getErr == nil {
			metrics.ObservePDUMismatch(msg.BilledSegments, pdu)
		}
		return nil
	}
	if isUniqueViolation(err) && pid != nil {
		_, err = q.UpdateSmsMessageFromStatistic(ctx, sqlcdb.UpdateSmsMessageFromStatisticParams{
			PduCount:  pdu,
			Delivered: row.Delivered,
			Sent:      row.Sent,
			ID:        id,
		})
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
	}
	return err
}

func markAccepted(ctx context.Context, q *sqlcdb.Queries, id uuid.UUID, pid *string) error {
	_, err := q.UpdateSmsMessageAccepted(ctx, sqlcdb.UpdateSmsMessageAcceptedParams{
		ProviderSmsID: pid,
		ID:            id,
	})
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if isUniqueViolation(err) && pid != nil {
		_, err = q.UpdateSmsMessageAccepted(ctx, sqlcdb.UpdateSmsMessageAcceptedParams{ID: id})
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
	}
	return err
}

func (w *Worker) noteSMSID(id string) {
	if id == "" {
		return
	}
	if w.statUsed == nil {
		w.statUsed = map[string]struct{}{}
	}
	w.statUsed[id] = struct{}{}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

func needStatistic(job sqlcdb.SendJob) bool {
	e := deref(job.LastError)
	return e == needStat || e == needRetry
}

func classify(err error) (sqlcdb.SendAttemptKind, int) {
	if err == nil {
		return sqlcdb.SendAttemptKindAccepted, 200
	}
	st := runexis.HTTPStatus(err)
	if st == 429 {
		return sqlcdb.SendAttemptKindRateLimited, st
	}
	if st >= 400 && st < 500 {
		return sqlcdb.SendAttemptKindRejected4xx, st
	}
	if st >= 500 {
		return sqlcdb.SendAttemptKind5xx, st
	}
	if runexis.IsTimeout(err) {
		return sqlcdb.SendAttemptKindTimeout, st
	}
	if st == 0 {
		return sqlcdb.SendAttemptKindTimeout, 0
	}
	return sqlcdb.SendAttemptKind5xx, st
}

func burst(rate float64) float64 {
	if rate < 0.1 {
		rate = 0.1
	}
	b := rate * 2
	if b < 1 {
		return 1
	}
	return b
}

func backoff429(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(attempt) * 2 * time.Second
	if d > time.Minute {
		return time.Minute
	}
	return d
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func Classify(err error) sqlcdb.SendAttemptKind {
	k, _ := classify(err)
	return k
}

func providerStatusForJob(ctx context.Context, q *sqlcdb.Queries, jobID uuid.UUID, fallback string) string {
	if q != nil {
		att, err := q.GetLatestSendAttempt(ctx, jobID)
		if err == nil {
			if s := ProviderStatusFromAttempt(att.HttpStatus, att.ResponseBody); s != "" {
				return s
			}
		}
	}
	return fallback
}

func ProviderStatusFromAttempt(httpStatus *int32, body *string) string {
	raw := strings.TrimSpace(deref(body))
	status := 0
	if httpStatus != nil {
		status = int(*httpStatus)
	}
	msg, reqID := extractProviderComplaint(raw)
	if msg == "" {
		return ""
	}
	msg = strings.TrimPrefix(msg, "runexis: ")
	if i := strings.Index(msg, " (request_id="); i > 0 {
		if reqID == "" {
			reqID = strings.TrimSuffix(msg[i+len(" (request_id="):], ")")
		}
		msg = msg[:i]
	}
	out := msg
	if status > 0 {
		out = fmt.Sprintf("runexis http %d: %s", status, msg)
	} else {
		out = "runexis: " + msg
	}
	if reqID != "" && !strings.Contains(out, reqID) {
		out += " (request_id=" + reqID + ")"
	}
	return out
}

func extractProviderComplaint(raw string) (msg, reqID string) {
	if raw == "" {
		return "", ""
	}
	var env struct {
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if json.Unmarshal([]byte(raw), &env) == nil && (env.Message != "" || env.RequestID != "") {
		return env.Message, env.RequestID
	}
	if strings.HasPrefix(raw, "{") {
		return "", ""
	}
	return raw, ""
}

func (w *Worker) capture(ctx context.Context, id uuid.UUID) {
	if w == nil || w.billing == nil {
		return
	}
	if err := w.billing.CaptureForMessage(ctx, id); err != nil && w.log != nil {
		w.log.Error("billing capture", "id", id, "err", err)
	}
}

func (w *Worker) release(ctx context.Context, id uuid.UUID) {
	if w == nil || w.billing == nil {
		return
	}
	if err := w.billing.ReleaseForMessage(ctx, id); err != nil && w.log != nil {
		w.log.Error("billing release", "id", id, "err", err)
	}
}

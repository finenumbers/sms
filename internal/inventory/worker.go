package inventory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/runexis"
)

const (
	maxDirectionAttempts = 15
	staleProcessingAfter = 2 * time.Minute
	notConfiguredDelay   = 30 * time.Second
	claimBatch           = 10
)

type DirectionWorker struct {
	store    *db.Store
	rx       *runexis.Client
	log      *slog.Logger
	ops      *ops.Logger
	workerID string
	now      func() time.Time
}

func NewDirectionWorker(store *db.Store, rx *runexis.Client, log *slog.Logger, opsLog *ops.Logger) *DirectionWorker {
	host, _ := os.Hostname()
	id := fmt.Sprintf("%s-%d-%s", host, os.Getpid(), uuid.NewString()[:8])
	if log == nil {
		log = slog.Default()
	}
	return &DirectionWorker{
		store:    store,
		rx:       rx,
		log:      log,
		ops:      opsLog,
		workerID: id,
		now:      func() time.Time { return time.Now().UTC() },
	}
}

func (w *DirectionWorker) Tick(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	stale := w.now().Add(-staleProcessingAfter)
	if _, err := w.store.Queries.ReclaimStaleDirectionJobs(ctx, &stale); err != nil {
		return 0, err
	}
	jobs, err := w.store.Queries.ClaimDirectionJobs(ctx, sqlcdb.ClaimDirectionJobsParams{
		WorkerID:   &w.workerID,
		BatchLimit: claimBatch,
	})
	if err != nil {
		return 0, err
	}
	for i := range jobs {
		w.process(ctx, jobs[i])
	}
	return len(jobs), nil
}

func (w *DirectionWorker) process(ctx context.Context, job sqlcdb.NumberDirectionJob) {
	ctx = ops.ContextWith(ctx, ops.Fields{
		RequestID:    "dir:" + job.ID.String(),
		ResourceType: "number_direction_job",
		ResourceID:   &job.ID,
	})
	if w.rx == nil {
		w.retry(ctx, job, false, "runexis adapter unavailable")
		return
	}
	err := w.rx.SetSMSDirections(ctx, job.Msisdn, runexis.SMSDirections{
		In:     job.DirIn,
		DomOut: job.DirDomOut,
		IntOut: job.DirIntOut,
		InMass: job.DirInMass,
	})
	if err == nil {
		if err := w.store.Queries.CompleteDirectionJob(ctx, job.ID); err != nil && w.log != nil {
			w.log.Error("complete direction job", "id", job.ID, "err", err)
		}
		return
	}
	if errors.Is(err, runexis.ErrNotConfigured) {
		w.retry(ctx, job, false, err.Error())
		return
	}
	var apiErr *runexis.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == 429 {
			w.retry(ctx, job, true, err.Error())
			return
		}
		if apiErr.Status >= 400 && apiErr.Status < 500 {
			w.dead(ctx, job, err.Error())
			return
		}
	}
	w.retry(ctx, job, true, err.Error())
}

func (w *DirectionWorker) retry(ctx context.Context, job sqlcdb.NumberDirectionJob, countAttempt bool, msg string) {
	attempt := job.Attempt
	if countAttempt {
		attempt++
	}
	if countAttempt && attempt >= maxDirectionAttempts {
		w.dead(ctx, job, msg)
		return
	}
	delay := notConfiguredDelay
	if countAttempt {
		delay = backoff(attempt)
	}
	err := w.store.Queries.RetryDirectionJob(ctx, sqlcdb.RetryDirectionJobParams{
		Attempt:     attempt,
		AvailableAt: w.now().Add(delay),
		LastError:   &msg,
		ID:          job.ID,
	})
	if err != nil && w.log != nil {
		w.log.Error("retry direction job", "id", job.ID, "err", err)
	}
}

func (w *DirectionWorker) dead(ctx context.Context, job sqlcdb.NumberDirectionJob, msg string) {
	attempt := job.Attempt + 1
	if err := w.store.Queries.DeadDirectionJob(ctx, sqlcdb.DeadDirectionJobParams{
		Attempt:   attempt,
		LastError: &msg,
		ID:        job.ID,
	}); err != nil {
		if w.log != nil {
			w.log.Error("dead direction job", "id", job.ID, "err", err)
		}
		return
	}
	if w.ops != nil {
		w.ops.Write(ctx, ops.Event{
			Category:     ops.CategoryQueue,
			Level:        ops.LevelError,
			Action:       "direction_job.dead",
			ResourceType: "number_direction_job",
			ResourceID:   &job.ID,
			Summary:      msg,
			Error:        msg,
			Detail: map[string]any{
				"msisdn":  job.Msisdn,
				"attempt": attempt,
			},
		})
	}
}

func backoff(attempt int32) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := time.Duration(5*attempt*attempt) * time.Second
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

package inventory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/runexis"
)

const (
	syncCap            = 500
	syncPageSize       = 30
	syncAccountWorkers = 4
	syncDeadline       = 55 * time.Second
	syncMaxErrors      = 50
)

type SyncReport struct {
	Fetched        int      `json:"fetched"`
	SMSOk          int      `json:"sms_ok"`
	Imported       int      `json:"imported"`
	Updated        int      `json:"updated"`
	SkippedNoSMS   int      `json:"skipped_no_sms"`
	SkippedInvalid int      `json:"skipped_invalid"`
	Truncated      bool     `json:"truncated"`
	Errors         []string `json:"errors"`
}

func (s *Service) SyncFromProvider(ctx context.Context) (SyncReport, error) {
	if s == nil || s.rx == nil {
		return SyncReport{}, runexis.ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, syncDeadline)
	defer cancel()

	rep := SyncReport{Errors: []string{}}
	page := 1
	maxPages := syncCap/syncPageSize + 2

	for page <= maxPages {
		if ctx.Err() != nil {
			rep.Truncated = true
			break
		}
		if rep.Fetched >= syncCap {
			rep.Truncated = true
			break
		}
		pg, err := s.rx.ListManagedNumbers(ctx, page, syncPageSize)
		if err != nil {
			if timedOut(err) {
				rep.Truncated = true
				break
			}
			if page == 1 && rep.Fetched == 0 {
				return SyncReport{}, err
			}
			appendSyncErr(&rep, err.Error())
			rep.Truncated = true
			break
		}
		if len(pg.Items) == 0 {
			break
		}
		items := pg.Items
		remain := syncCap - rep.Fetched
		if remain <= 0 {
			rep.Truncated = true
			break
		}
		if len(items) > remain {
			items = items[:remain]
			rep.Truncated = true
		}
		s.syncPage(ctx, items, &rep)
		if ctx.Err() != nil {
			rep.Truncated = true
			break
		}
		limit := pg.Limit
		if limit <= 0 {
			limit = syncPageSize
		}
		if pg.Total > 0 && page*limit >= pg.Total {
			break
		}
		if len(pg.Items) < limit {
			break
		}
		page++
	}
	if page > maxPages {
		rep.Truncated = true
	}
	if rep.Errors == nil {
		rep.Errors = []string{}
	}
	return rep, nil
}

func (s *Service) syncPage(ctx context.Context, items []runexis.ManagedNumber, rep *SyncReport) {
	sem := make(chan struct{}, syncAccountWorkers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, item := range items {
		if ctx.Err() != nil {
			mu.Lock()
			rep.Truncated = true
			mu.Unlock()
			break
		}
		item := item
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			s.syncOne(ctx, item, rep, &mu)
		}()
	}
	wg.Wait()
}

func (s *Service) syncOne(ctx context.Context, item runexis.ManagedNumber, rep *SyncReport, mu *sync.Mutex) {
	mu.Lock()
	rep.Fetched++
	mu.Unlock()
	if ctx.Err() != nil {
		mu.Lock()
		rep.Truncated = true
		mu.Unlock()
		return
	}

	n, err := msisdn.FromManagement(item.Code, item.Number)
	if err != nil {
		mu.Lock()
		rep.SkippedInvalid++
		appendSyncErr(rep, fmt.Sprintf("%s%s: invalid msisdn", item.Code, item.Number))
		mu.Unlock()
		return
	}

	_, err = s.rx.SMSAccount(ctx, n)
	if err != nil {
		if timedOut(err) {
			mu.Lock()
			rep.Truncated = true
			mu.Unlock()
			return
		}
		if runexis.IsNoSMS(err) {
			s.markNoSMS(ctx, n, item.Snapshot, rep, mu)
			return
		}
		mu.Lock()
		appendSyncErr(rep, fmt.Sprintf("%s: %s", n, err.Error()))
		mu.Unlock()
		return
	}

	_, getErr := s.store.Queries.GetDefNumberByMSISDN(ctx, n)
	inserted := errors.Is(getErr, pgx.ErrNoRows)
	if getErr != nil && !inserted {
		mu.Lock()
		appendSyncErr(rep, fmt.Sprintf("%s: %s", n, getErr.Error()))
		mu.Unlock()
		return
	}
	if _, err := s.store.Queries.UpsertDefNumberFromSync(ctx, sqlcdb.UpsertDefNumberFromSyncParams{
		Msisdn:          n,
		Region:          optString(item.CityName),
		SupportsSms:     true,
		RunexisSnapshot: snapshotBytes(item.Snapshot),
	}); err != nil {
		mu.Lock()
		appendSyncErr(rep, fmt.Sprintf("%s: %s", n, err.Error()))
		mu.Unlock()
		return
	}
	mu.Lock()
	rep.SMSOk++
	if inserted {
		rep.Imported++
	} else {
		rep.Updated++
	}
	mu.Unlock()
}

func (s *Service) markNoSMS(ctx context.Context, n string, snapshot []byte, rep *SyncReport, mu *sync.Mutex) {
	_, err := s.store.Queries.GetDefNumberByMSISDN(ctx, n)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			mu.Lock()
			appendSyncErr(rep, fmt.Sprintf("%s: %s", n, err.Error()))
			mu.Unlock()
			return
		}
		mu.Lock()
		rep.SkippedNoSMS++
		mu.Unlock()
		return
	}
	if _, err := s.store.Queries.SetDefNumberSupportsSMSByMSISDN(ctx, sqlcdb.SetDefNumberSupportsSMSByMSISDNParams{
		SupportsSms:     false,
		RunexisSnapshot: snapshotBytes(snapshot),
		Msisdn:          n,
	}); err != nil {
		mu.Lock()
		appendSyncErr(rep, fmt.Sprintf("%s: %s", n, err.Error()))
		mu.Unlock()
		return
	}
	mu.Lock()
	rep.SkippedNoSMS++
	mu.Unlock()
}

func snapshotBytes(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func timedOut(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func appendSyncErr(rep *SyncReport, msg string) {
	if len(rep.Errors) >= syncMaxErrors {
		return
	}
	rep.Errors = append(rep.Errors, msg)
}

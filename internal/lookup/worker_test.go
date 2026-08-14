package lookup

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"

	"finenumbers/sms/internal/billing"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

func TestRefreshSMSCBalanceNoopsWithoutCache(t *testing.T) {
	w := &Worker{now: func() time.Time { return time.Now().UTC() }}
	w.RefreshSMSCBalance(context.Background())
}

func TestExistingJobOnUniqueRequiresViolation(t *testing.T) {
	s := &Service{}
	if _, ok := s.existingJobOnUnique(context.Background(), nil, uuid.Nil, "k", errors.New("nope")); ok {
		t.Fatal("non-unique must not treat as replay")
	}
	uniq := &pgconn.PgError{Code: pgerrcode.UniqueViolation}
	if _, ok := s.existingJobOnUnique(context.Background(), nil, uuid.Nil, "", uniq); ok {
		t.Fatal("empty idempotency key must not treat as replay")
	}
}

func TestCSVConsumingHealOutlivesCreateTimeout(t *testing.T) {
	if csvConsumingAge <= createStatementTimeout {
		t.Fatalf("csvConsumingAge %s must exceed createStatementTimeout %s", csvConsumingAge, createStatementTimeout)
	}
}

func TestCanStartLookupIO(t *testing.T) {
	now := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	if !canStartLookupIO(now, time.Time{}) {
		t.Fatal("zero deadline means no Tick budget (callback / tests)")
	}
	if !canStartLookupIO(now, now.Add(minHTTPBudget)) {
		t.Fatal("exactly min budget must start")
	}
	if canStartLookupIO(now, now.Add(minHTTPBudget-time.Millisecond)) {
		t.Fatal("below min budget must not start another SMSC call")
	}
	if canStartLookupIO(now, now.Add(-time.Second)) {
		t.Fatal("expired deadline must not start I/O")
	}
}

func TestLookupIOContextCapsWorkerCallsOnly(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	plain, stop := lookupIOContext(parent, time.Time{})
	stop()
	if plain != parent {
		t.Fatal("callback path must not wrap ctx")
	}
	capped, stop := lookupIOContext(parent, time.Now().Add(time.Minute))
	defer stop()
	dl, ok := capped.Deadline()
	if !ok || time.Until(dl) > smscCallTimeout+50*time.Millisecond {
		t.Fatal("worker SMSC call must be capped")
	}
}

func TestIsRetryableLookupIO(t *testing.T) {
	if !isRetryableLookupIO(context.DeadlineExceeded) || !isRetryableLookupIO(context.Canceled) {
		t.Fatal("tick deadline must leave the item reserved, not RELEASE")
	}
	if !isRetryableLookupIO(&smsc.Error{Retryable: true, Kind: smsc.KindTimeout}) {
		t.Fatal("retryable provider error")
	}
	if isRetryableLookupIO(&smsc.Error{Retryable: false, Kind: smsc.KindValidation}) {
		t.Fatal("non-retryable must fail the item")
	}
	if isRetryableLookupIO(errors.New("other")) {
		t.Fatal("unknown errors are not silently skipped")
	}
}

func TestOnItemTerminalUsesSettleAction(t *testing.T) {
	reachable := "reachable"
	item := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusCompleted,
		ResultStatus: &reachable,
	}
	if billing.LookupItemSettleAction(item) != "capture" {
		t.Fatal("provider-final completed must capture")
	}
	failed := sqlcdb.LookupItem{Status: sqlcdb.LookupItemStatusFailed}
	if billing.LookupItemSettleAction(failed) != "release" {
		t.Fatal("failed without provider result must release")
	}
	errStatus := "error"
	failedFinal := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusFailed,
		ResultStatus: &errStatus,
	}
	if billing.LookupItemSettleAction(failedFinal) != "capture" {
		t.Fatal("Policy B: provider-final error captures")
	}
}

func TestBumpEnrichAttemptSkipsBudgetMiss(t *testing.T) {
	if !bumpEnrichAttempt(true, true) {
		t.Fatal("real SMSC call must count even if the tick budget then expires")
	}
	if !bumpEnrichAttempt(true, false) {
		t.Fatal("real SMSC call must count")
	}
	if bumpEnrichAttempt(false, true) {
		t.Fatal("budget expiry without SMSC must not burn an enrich attempt")
	}
	if !bumpEnrichAttempt(false, false) {
		t.Fatal("claimed item that cannot be enriched must still advance attempts")
	}
}

func TestTransitionOmitsBillingAction(t *testing.T) {
	var arg sqlcdb.TransitionLookupItemParams
	if _, ok := reflect.TypeOf(arg).FieldByName("BillingAction"); ok {
		t.Fatal("TransitionLookupItem must not write billing_action; use SetLookupItemBillingAction")
	}
}

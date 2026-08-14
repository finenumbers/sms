package billing

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestProductForCheckType(t *testing.T) {
	p, err := ProductForCheckType(sqlcdb.LookupCheckTypeHlr)
	if err != nil || p != sqlcdb.BillingProductHlr {
		t.Fatalf("hlr: %s %v", p, err)
	}
	p, err = ProductForCheckType(sqlcdb.LookupCheckTypePing)
	if err != nil || p != sqlcdb.BillingProductSilentSms {
		t.Fatalf("ping: %s %v", p, err)
	}
	if _, err := ProductForCheckType("sms"); err == nil {
		t.Fatal("expected invalid check type")
	}
}

func TestKnownProduct(t *testing.T) {
	if !KnownProduct(sqlcdb.BillingProductSmsDomestic) || !KnownProduct(sqlcdb.BillingProductHlr) || !KnownProduct(sqlcdb.BillingProductSilentSms) {
		t.Fatal("expected known products")
	}
	if KnownProduct("nope") {
		t.Fatal("unknown product")
	}
}

func TestRemainingOnHold(t *testing.T) {
	hold := decimal.RequireFromString("10")
	left, err := RemainingOnHold(hold, decimal.RequireFromString("4"))
	if err != nil || !left.Equal(decimal.RequireFromString("6")) {
		t.Fatalf("got %s %v", left, err)
	}
	if HoldIsOpen(decimal.Zero) {
		t.Fatal("zero remaining is closed")
	}
	if !HoldIsOpen(left) {
		t.Fatal("expected open")
	}
	if _, err := RemainingOnHold(hold, decimal.RequireFromString("11")); err == nil {
		t.Fatal("expected over-settle")
	}
}

func TestPartialHoldMathDoesNotCloseOnFirstChild(t *testing.T) {
	hold := decimal.RequireFromString("6")
	share := decimal.RequireFromString("2")
	settled := decimal.Zero
	for i := 0; i < 2; i++ {
		left, err := RemainingOnHold(hold, settled)
		if err != nil || !HoldIsOpen(left) {
			t.Fatalf("step %d remaining %s %v", i, left, err)
		}
		if left.LessThan(share) {
			t.Fatalf("step %d cannot take share", i)
		}
		settled = settled.Add(share)
	}
	left, err := RemainingOnHold(hold, settled)
	if err != nil || !left.Equal(share) || !HoldIsOpen(left) {
		t.Fatalf("after two shares remaining %s %v", left, err)
	}
	settled = settled.Add(share)
	left, err = RemainingOnHold(hold, settled)
	if err != nil || HoldIsOpen(left) {
		t.Fatalf("fully settled %s %v", left, err)
	}
}

func TestLookupItemSettleActionPolicyB(t *testing.T) {
	reachable := "reachable"
	unreachable := "unreachable"
	completed := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusCompleted,
		ResultStatus: &reachable,
	}
	if LookupItemSettleAction(completed) != "capture" {
		t.Fatal("completed reachable must capture")
	}
	completed.ResultStatus = &unreachable
	if LookupItemSettleAction(completed) != "capture" {
		t.Fatal("completed unreachable must capture (Policy B)")
	}
	failedTimeout := sqlcdb.LookupItem{Status: sqlcdb.LookupItemStatusFailed}
	if LookupItemSettleAction(failedTimeout) != "release" {
		t.Fatal("failed without provider-final result must release")
	}
	errStatus := "error"
	failedFinal := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusFailed,
		ResultStatus: &errStatus,
	}
	if LookupItemSettleAction(failedFinal) != "capture" {
		t.Fatal("provider-final error must capture")
	}
	already := sqlcdb.LookupItem{
		Status: sqlcdb.LookupItemStatusFailed,
		BillingAction: sqlcdb.NullBillingAction{
			BillingAction: sqlcdb.BillingActionRelease,
			Valid:         true,
		},
	}
	if LookupItemSettleAction(already) != "release" {
		t.Fatal("persisted billing_action wins")
	}
}

func TestLookupRemainderBlockedUntilItemsPosted(t *testing.T) {
	if !LookupRemainderBlocked(1) {
		t.Fatal("one unposted or open item must block remainder")
	}
	if !LookupRemainderBlocked(3) {
		t.Fatal("several blocking items must block remainder")
	}
	if LookupRemainderBlocked(0) {
		t.Fatal("fully posted terminal job may release remainder")
	}
}

func TestLookupSettleActionDoesNotTreatEmptyFlagAsPosted(t *testing.T) {
	reachable := "reachable"
	item := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusCompleted,
		ResultStatus: &reachable,
	}
	if LookupItemSettleAction(item) != "capture" {
		t.Fatal("completed without billing_action is still unsettled capture intent")
	}
	item.BillingAction = sqlcdb.NullBillingAction{}
	if item.BillingAction.Valid {
		t.Fatal("empty flag must not look posted")
	}
}

func TestHoldTimesNRemainingAfterPartialSettle(t *testing.T) {
	unit := decimal.RequireFromString("1.50")
	hold := unit.Mul(decimal.NewFromInt(3))
	if !hold.Equal(decimal.RequireFromString("4.50")) {
		t.Fatalf("HOLD×N %s", hold)
	}
	afterOneCapture, err := RemainingOnHold(hold, unit)
	if err != nil || !afterOneCapture.Equal(decimal.RequireFromString("3.00")) {
		t.Fatalf("after one capture %s %v", afterOneCapture, err)
	}
	afterTwo, err := RemainingOnHold(hold, unit.Add(unit))
	if err != nil || !afterTwo.Equal(unit) {
		t.Fatalf("after two %s %v", afterTwo, err)
	}
	closed, err := RemainingOnHold(hold, hold)
	if err != nil || HoldIsOpen(closed) {
		t.Fatalf("fully settled %s %v", closed, err)
	}
}

func TestLateCallbackAfterReleaseHasNoRecaptureIntent(t *testing.T) {
	item := sqlcdb.LookupItem{
		Status: sqlcdb.LookupItemStatusFailed,
		BillingAction: sqlcdb.NullBillingAction{
			BillingAction: sqlcdb.BillingActionRelease,
			Valid:         true,
		},
	}
	if LookupItemSettleAction(item) != "release" {
		t.Fatal("posted RELEASE must win over a later provider-final payload")
	}
}

func TestLookupIdempotencyKeys(t *testing.T) {
	id := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	if LookupHoldKey(id) != "hold:lookup_job:"+id.String() {
		t.Fatal(LookupHoldKey(id))
	}
	if LookupDebitKey(id) != "debit:lookup_item:"+id.String() {
		t.Fatal(LookupDebitKey(id))
	}
}

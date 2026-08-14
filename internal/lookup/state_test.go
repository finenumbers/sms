package lookup

import (
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

func TestItemTransitions(t *testing.T) {
	if !CanTransitionItem(sqlcdb.LookupItemStatusQueued, sqlcdb.LookupItemStatusReserved) {
		t.Fatal("queued→reserved")
	}
	if !CanTransitionItem(sqlcdb.LookupItemStatusReserved, sqlcdb.LookupItemStatusPending) {
		t.Fatal("reserved→pending")
	}
	if !CanTransitionItem(sqlcdb.LookupItemStatusPending, sqlcdb.LookupItemStatusCompleted) {
		t.Fatal("pending→completed")
	}
	if CanTransitionItem(sqlcdb.LookupItemStatusCompleted, sqlcdb.LookupItemStatusPending) {
		t.Fatal("completed must not go back")
	}
	if CanTransitionItem(sqlcdb.LookupItemStatusQueued, sqlcdb.LookupItemStatusPending) {
		t.Fatal("queued→pending is illegal (no SENT skip from queued in our table; submit claims reserved first)")
	}
}

func TestMapLifecycle(t *testing.T) {
	got, ok := MapLifecycleToItemStatus(smsc.LifecycleCompleted, sqlcdb.LookupItemStatusPending)
	if !ok || got != sqlcdb.LookupItemStatusCompleted {
		t.Fatal(got, ok)
	}
	got, ok = MapLifecycleToItemStatus(smsc.LifecycleFailed, sqlcdb.LookupItemStatusPending)
	if !ok || got != sqlcdb.LookupItemStatusFailed {
		t.Fatal(got, ok)
	}
	got, ok = MapLifecycleToItemStatus(smsc.LifecycleAccepted, sqlcdb.LookupItemStatusReserved)
	if !ok || got != sqlcdb.LookupItemStatusPending {
		t.Fatal(got, ok)
	}
}

func TestDeriveJobTerminalStatus(t *testing.T) {
	if DeriveJobTerminalStatus(2, 2, 0) != sqlcdb.LookupJobStatusCompleted {
		t.Fatal("completed")
	}
	if DeriveJobTerminalStatus(2, 0, 2) != sqlcdb.LookupJobStatusFailed {
		t.Fatal("failed")
	}
	if DeriveJobTerminalStatus(2, 1, 1) != sqlcdb.LookupJobStatusCompletedWithErrors {
		t.Fatal("with errors")
	}
}

func TestComputeProgress(t *testing.T) {
	p := ComputeProgress(10, 3, 2)
	if p.Total != 10 || p.Processed != 5 || p.Success != 3 || p.Failed != 2 || p.Pending != 5 {
		t.Fatalf("%#v", p)
	}
}

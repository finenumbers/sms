package lookup

import (
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/smsc"
)

var itemTransitions = map[sqlcdb.LookupItemStatus][]sqlcdb.LookupItemStatus{
	sqlcdb.LookupItemStatusQueued: {
		sqlcdb.LookupItemStatusReserved,
		sqlcdb.LookupItemStatusFailed,
	},
	sqlcdb.LookupItemStatusReserved: {
		sqlcdb.LookupItemStatusPending,
		sqlcdb.LookupItemStatusCompleted,
		sqlcdb.LookupItemStatusFailed,
	},
	sqlcdb.LookupItemStatusPending: {
		sqlcdb.LookupItemStatusCompleted,
		sqlcdb.LookupItemStatusFailed,
	},
	sqlcdb.LookupItemStatusCompleted: {},
	sqlcdb.LookupItemStatusFailed:    {},
}

func IsTerminalItem(status sqlcdb.LookupItemStatus) bool {
	return status == sqlcdb.LookupItemStatusCompleted || status == sqlcdb.LookupItemStatusFailed
}

func IsTerminalJob(status sqlcdb.LookupJobStatus) bool {
	return status == sqlcdb.LookupJobStatusCompleted ||
		status == sqlcdb.LookupJobStatusCompletedWithErrors ||
		status == sqlcdb.LookupJobStatusFailed
}

func itemStatusFilter(ss ...sqlcdb.LookupItemStatus) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = string(s)
	}
	return out
}

func CanTransitionItem(from, to sqlcdb.LookupItemStatus) bool {
	if from == to {
		return true
	}
	for _, next := range itemTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

func MapLifecycleToItemStatus(lifecycle smsc.Lifecycle, current sqlcdb.LookupItemStatus) (sqlcdb.LookupItemStatus, bool) {
	switch lifecycle {
	case smsc.LifecycleAccepted, smsc.LifecyclePending:
		switch current {
		case sqlcdb.LookupItemStatusReserved, sqlcdb.LookupItemStatusQueued, sqlcdb.LookupItemStatusPending:
			return sqlcdb.LookupItemStatusPending, true
		default:
			return "", false
		}
	case smsc.LifecycleCompleted:
		return sqlcdb.LookupItemStatusCompleted, true
	case smsc.LifecycleFailed:
		return sqlcdb.LookupItemStatusFailed, true
	default:
		return "", false
	}
}

func DeriveJobTerminalStatus(total, success, failed int32) sqlcdb.LookupJobStatus {
	if total == 0 {
		return sqlcdb.LookupJobStatusFailed
	}
	if failed == 0 && success == total {
		return sqlcdb.LookupJobStatusCompleted
	}
	if success == 0 && failed == total {
		return sqlcdb.LookupJobStatusFailed
	}
	return sqlcdb.LookupJobStatusCompletedWithErrors
}

type Progress struct {
	Total     int32
	Processed int32
	Success   int32
	Failed    int32
	Pending   int32
}

func ComputeProgress(itemCount, successCount, failureCount int32) Progress {
	processed := successCount + failureCount
	pending := itemCount - processed
	if pending < 0 {
		pending = 0
	}
	return Progress{
		Total:     itemCount,
		Processed: processed,
		Success:   successCount,
		Failed:    failureCount,
		Pending:   pending,
	}
}

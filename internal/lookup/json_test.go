package lookup

import (
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

func TestItemJSONErrorLabel(t *testing.T) {
	t.Parallel()
	completed := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusCompleted,
		ResultStatus: strPtr("reachable"),
		ErrorCode:    strPtr("0"),
	}
	got := ItemJSON(completed)
	if got["error_message"] != "Нет ошибки" {
		t.Fatalf("err=0: %#v", got["error_message"])
	}

	completed.ResultStatus = strPtr("unreachable")
	completed.ErrorCode = strPtr("6")
	got = ItemJSON(completed)
	if got["error_message"] != "Абонент не в сети" {
		t.Fatalf("err=6: %#v", got["error_message"])
	}

	failed := sqlcdb.LookupItem{
		Status:       sqlcdb.LookupItemStatusFailed,
		ErrorCode:    strPtr("check_timeout"),
		ErrorMessage: strPtr("Истекло время ожидания ответа провайдера"),
	}
	got = ItemJSON(failed)
	if got["error_message"] == nil || *got["error_message"].(*string) != "Истекло время ожидания ответа провайдера" {
		t.Fatalf("failed keeps raw: %#v", got["error_message"])
	}

	empty := sqlcdb.LookupItem{Status: sqlcdb.LookupItemStatusPending}
	got = ItemJSON(empty)
	if got["error_message"] != nil {
		t.Fatalf("no code: %#v", got["error_message"])
	}
}

func TestJobJSONReachabilityCounters(t *testing.T) {
	t.Parallel()
	got := JobJSON(sqlcdb.LookupJob{
		ItemCount:        4,
		SuccessCount:     3,
		FailureCount:     1,
		ReachableCount:   2,
		UnreachableCount: 1,
		Currency:         "RUB",
	})
	if got["reachable_count"] != int32(2) || got["unreachable_count"] != int32(1) {
		t.Fatalf("%#v", got)
	}
}

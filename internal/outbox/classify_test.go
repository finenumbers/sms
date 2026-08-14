package outbox

import (
	"context"
	"errors"
	"net"
	"testing"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/runexis"
)

func TestClassify(t *testing.T) {
	if Classify(nil) != sqlcdb.SendAttemptKindAccepted {
		t.Fatal("nil")
	}
	if Classify(&runexis.APIError{Status: 400}) != sqlcdb.SendAttemptKindRejected4xx {
		t.Fatal("400")
	}
	if Classify(&runexis.APIError{Status: 429}) != sqlcdb.SendAttemptKindRateLimited {
		t.Fatal("429")
	}
	if Classify(&runexis.APIError{Status: 503}) != sqlcdb.SendAttemptKind5xx {
		t.Fatal("503")
	}
	if Classify(context.DeadlineExceeded) != sqlcdb.SendAttemptKindTimeout {
		t.Fatal("deadline")
	}
	if Classify(&net.DNSError{IsTimeout: true, Err: "timeout"}) != sqlcdb.SendAttemptKindTimeout {
		t.Fatal("net timeout")
	}
	if Classify(errors.New("boom")) != sqlcdb.SendAttemptKindTimeout {
		t.Fatal("unknown net-less error treated as timeout/uncertain")
	}
}

func TestProviderStatusFromAttempt(t *testing.T) {
	st := int32(500)
	body := `{"success":false,"message":"an unexpected error has occurred","request_id":"req-abc"}`
	got := ProviderStatusFromAttempt(&st, &body)
	if got != "runexis http 500: an unexpected error has occurred (request_id=req-abc)" {
		t.Fatalf("%q", got)
	}
	plain := "runexis: an unexpected error has occurred (request_id=req-xyz)"
	got = ProviderStatusFromAttempt(&st, &plain)
	if got != "runexis http 500: an unexpected error has occurred (request_id=req-xyz)" {
		t.Fatalf("plain %q", got)
	}
	if ProviderStatusFromAttempt(nil, nil) != "" {
		t.Fatal("empty")
	}
}

func TestAllowUncertainRetry(t *testing.T) {
	if allowUncertainRetry(1, sqlcdb.SendAttemptKind5xx) {
		t.Fatal("JSON/HTTP 5xx must not send a second POST")
	}
	if !allowUncertainRetry(1, sqlcdb.SendAttemptKindTimeout) {
		t.Fatal("timeout may retry once after statistic miss")
	}
	if !allowUncertainRetry(1, "") {
		t.Fatal("reclaim without attempt row may retry")
	}
	if allowUncertainRetry(2, sqlcdb.SendAttemptKindTimeout) {
		t.Fatal("maxUncertainSends")
	}
}

func TestNeedStatistic(t *testing.T) {
	s := needStat
	if !needStatistic(sqlcdb.SendJob{LastError: &s}) {
		t.Fatal("need_stat")
	}
	r := needRetry
	if !needStatistic(sqlcdb.SendJob{LastError: &r}) {
		t.Fatal("need_retry")
	}
	if needStatistic(sqlcdb.SendJob{}) {
		t.Fatal("empty")
	}
}

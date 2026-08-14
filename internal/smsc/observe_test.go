package smsc

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"finenumbers/sms/internal/metrics"
)

func TestObserveOutcomeCountsErrorCode9(t *testing.T) {
	beforeReq := testutil.ToFloat64(metrics.SMSCRequests.WithLabelValues("send", "failed"))
	before9 := testutil.ToFloat64(metrics.SMSCErrorCode9)
	observeOutcome(KindSend, &Error{ProviderErrorCode: 9, Kind: KindRateLimit})
	if got := testutil.ToFloat64(metrics.SMSCRequests.WithLabelValues("send", "failed")); got != beforeReq+1 {
		t.Fatalf("requests %v -> %v", beforeReq, got)
	}
	if got := testutil.ToFloat64(metrics.SMSCErrorCode9); got != before9+1 {
		t.Fatalf("error 9 %v -> %v", before9, got)
	}
}

func TestObserveOutcomeSuccessSkipsError9(t *testing.T) {
	before9 := testutil.ToFloat64(metrics.SMSCErrorCode9)
	observeOutcome(KindBalance, nil)
	if got := testutil.ToFloat64(metrics.SMSCErrorCode9); got != before9 {
		t.Fatal("success must not increment error_code 9")
	}
}

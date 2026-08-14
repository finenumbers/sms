package runexis

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"finenumbers/sms/internal/ops"
)

func TestSkipReconcileStatisticOps(t *testing.T) {
	rec := ops.ContextWith(context.Background(), ops.Fields{RequestID: "reconcile"})
	job := ops.ContextWith(context.Background(), ops.Fields{RequestID: "job:abc"})
	stat := "/api/v1/sms/statistic"
	if !skipReconcileStatisticOps(rec, http.MethodGet, stat, 200, nil) {
		t.Fatal("heartbeat 200 must be skipped")
	}
	if skipReconcileStatisticOps(rec, http.MethodGet, stat, 500, errors.New("boom")) {
		t.Fatal("reconcile 5xx must be logged")
	}
	if skipReconcileStatisticOps(job, http.MethodGet, stat, 200, nil) {
		t.Fatal("job statistic must be logged")
	}
	if skipReconcileStatisticOps(rec, http.MethodPost, "/api/v1/sms/send", 200, nil) {
		t.Fatal("send must be logged")
	}
	if skipReconcileStatisticOps(context.Background(), http.MethodGet, stat, 200, nil) {
		t.Fatal("statistic without reconcile id must be logged")
	}
}

package metrics

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

type stubStore struct{}

func (stubStore) CountSendJobsByStatus(context.Context) ([]sqlcdb.CountSendJobsByStatusRow, error) {
	return nil, nil
}
func (stubStore) CountSmsMessagesByStatus(context.Context) ([]sqlcdb.CountSmsMessagesByStatusRow, error) {
	return nil, nil
}
func (stubStore) CountUnprocessedCallbacks(context.Context) (int64, error) { return 0, nil }
func (stubStore) SumWalletBalances(context.Context) (sqlcdb.SumWalletBalancesRow, error) {
	return sqlcdb.SumWalletBalancesRow{}, nil
}
func (stubStore) CountOpenHolds(context.Context) (int64, error) { return 0, nil }
func (stubStore) CountLookupItemsByStatus(context.Context) ([]sqlcdb.CountLookupItemsByStatusRow, error) {
	return []sqlcdb.CountLookupItemsByStatusRow{{Status: sqlcdb.LookupItemStatusQueued, N: 2}}, nil
}
func (stubStore) CountLookupJobsByStatus(context.Context) ([]sqlcdb.CountLookupJobsByStatusRow, error) {
	return nil, nil
}
func (stubStore) CountOpenLookupHolds(context.Context) (int64, error) { return 4, nil }
func (stubStore) OldestUnprocessedLookupCallbackAt(context.Context) (time.Time, error) {
	return time.Time{}, nil
}

type stubRuntime struct {
	enabled    bool
	configured bool
	balance    float64
	hasBalance bool
}

func (s stubRuntime) LookupEnabled(context.Context) bool  { return s.enabled }
func (s stubRuntime) SMSCConfigured(context.Context) bool { return s.configured }
func (s stubRuntime) SMSCBalance(context.Context) (float64, bool) {
	return s.balance, s.hasBalance
}

func TestLookupHoldsNotMixedWithSMSHolds(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(StoreCollector{Stats: stubStore{}})
	body := gather(t, reg)
	if !strings.Contains(body, "fn_lookup_holds_open 4") {
		t.Fatalf("lookup holds missing:\n%s", body)
	}
	if !strings.Contains(body, `fn_lookup_items{status="queued"} 2`) {
		t.Fatalf("lookup items missing:\n%s", body)
	}
}

func TestSMSCBalanceOmittedWhenCacheMiss(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(LookupRuntimeCollector{Runtime: stubRuntime{enabled: false, configured: false}})
	body := gather(t, reg)
	if strings.Contains(body, "fn_smsc_balance") {
		t.Fatalf("balance must be absent without cache:\n%s", body)
	}
	if !strings.Contains(body, "fn_lookup_enabled 0") || !strings.Contains(body, "fn_smsc_configured 0") {
		t.Fatalf("flags:\n%s", body)
	}
}

func TestSMSCBalanceEmittedWhenCached(t *testing.T) {
	reg := prometheus.NewRegistry()
	reg.MustRegister(LookupRuntimeCollector{Runtime: stubRuntime{configured: true, hasBalance: true, balance: 81.5}})
	body := gather(t, reg)
	if !strings.Contains(body, "fn_smsc_balance 81.5") {
		t.Fatalf("balance:\n%s", body)
	}
}

func gather(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	srv := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer srv.Close()
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

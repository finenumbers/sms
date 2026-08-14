package metrics

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const ns = "fn"

var (
	HTTPRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "http_requests_total",
		Help:      "HTTP requests by surface, method, and status",
	}, []string{"surface", "method", "code"})

	HTTPDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: ns,
		Name:      "http_request_duration_seconds",
		Help:      "HTTP request duration",
		Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	}, []string{"surface", "method"})

	Callbacks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "callback_events_total",
		Help:      "Normalized provider callbacks",
	}, []string{"kind", "result"})

	RetentionDeleted = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "retention_deleted_total",
		Help:      "Rows deleted by retention",
	}, []string{"table"})

	PDUMismatch = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "billing_pdu_mismatch_total",
		Help:      "Provider pdu_count differs from billed_segments snapshot",
	})

	SMSCRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "smsc_requests_total",
		Help:      "SMSC adapter calls by kind and outcome",
	}, []string{"kind", "status"})

	SMSCErrorCode9 = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: ns,
		Name:      "smsc_error_code_9_total",
		Help:      "SMSC error_code=9 (duplicate flood / rate limit)",
	})
)

func init() {
	prometheus.MustRegister(HTTPRequests, HTTPDuration, Callbacks, RetentionDeleted, PDUMismatch, SMSCRequests, SMSCErrorCode9)
	prometheus.MustRegister(collectors.NewBuildInfoCollector())
}

func ObserveSMSCRequest(kind, status string) {
	if kind == "" {
		kind = "unknown"
	}
	if status == "" {
		status = "failed"
	}
	SMSCRequests.WithLabelValues(kind, status).Inc()
}

func ObserveSMSCErrorCode9() {
	SMSCErrorCode9.Inc()
}

func Handler() http.Handler {
	return promhttp.Handler()
}

var registerStoreOnce sync.Once

func RegisterStore(stats StoreStats) {
	if stats == nil {
		return
	}
	registerStoreOnce.Do(func() {
		prometheus.MustRegister(StoreCollector{Stats: stats})
	})
}

func ObserveHTTP(path, method string, status int, dur time.Duration) {
	if path == "/metrics" {
		return
	}
	surface := Surface(path)
	HTTPRequests.WithLabelValues(surface, method, strconv.Itoa(status)).Inc()
	HTTPDuration.WithLabelValues(surface, method).Observe(dur.Seconds())
}

func Surface(path string) string {
	switch {
	case path == "/healthz" || path == "/readyz":
		return "health"
	case path == "/metrics":
		return "metrics"
	case strings.HasPrefix(path, "/admin/v1"):
		return "admin"
	case strings.HasPrefix(path, "/client/v1"):
		return "client"
	case strings.HasPrefix(path, "/v1"):
		return "public"
	case strings.HasPrefix(path, "/internal/runexis"), strings.HasPrefix(path, "/internal/smsc"):
		return "ingress"
	default:
		return "other"
	}
}

type StoreStats interface {
	CountSendJobsByStatus(ctx context.Context) ([]sqlcdb.CountSendJobsByStatusRow, error)
	CountSmsMessagesByStatus(ctx context.Context) ([]sqlcdb.CountSmsMessagesByStatusRow, error)
	CountUnprocessedCallbacks(ctx context.Context) (int64, error)
	SumWalletBalances(ctx context.Context) (sqlcdb.SumWalletBalancesRow, error)
	CountOpenHolds(ctx context.Context) (int64, error)
	CountLookupItemsByStatus(ctx context.Context) ([]sqlcdb.CountLookupItemsByStatusRow, error)
	CountLookupJobsByStatus(ctx context.Context) ([]sqlcdb.CountLookupJobsByStatusRow, error)
	CountOpenLookupHolds(ctx context.Context) (int64, error)
	OldestUnprocessedLookupCallbackAt(ctx context.Context) (*time.Time, error)
}

type LookupRuntime interface {
	LookupEnabled(ctx context.Context) bool
	SMSCConfigured(ctx context.Context) bool
	SMSCBalance(ctx context.Context) (float64, bool)
}

var registerLookupOnce sync.Once

func RegisterLookupRuntime(rt LookupRuntime) {
	if rt == nil {
		return
	}
	registerLookupOnce.Do(func() {
		prometheus.MustRegister(LookupRuntimeCollector{Runtime: rt})
	})
}

type StoreCollector struct {
	Stats StoreStats
}

func (c StoreCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- jobsDesc
	ch <- messagesDesc
	ch <- callbacksDesc
	ch <- walletAvailDesc
	ch <- walletHeldDesc
	ch <- openHoldsDesc
	ch <- lookupItemsDesc
	ch <- lookupJobsDesc
	ch <- lookupHoldsDesc
	ch <- smscCallbackLagDesc
}

func (c StoreCollector) Collect(ch chan<- prometheus.Metric) {
	if c.Stats == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if rows, err := c.Stats.CountSendJobsByStatus(ctx); err == nil {
		for _, row := range rows {
			ch <- prometheus.MustNewConstMetric(jobsDesc, prometheus.GaugeValue, float64(row.N), string(row.Status))
		}
	}
	if rows, err := c.Stats.CountSmsMessagesByStatus(ctx); err == nil {
		for _, row := range rows {
			ch <- prometheus.MustNewConstMetric(messagesDesc, prometheus.GaugeValue, float64(row.N), string(row.Status))
		}
	}
	if n, err := c.Stats.CountUnprocessedCallbacks(ctx); err == nil {
		ch <- prometheus.MustNewConstMetric(callbacksDesc, prometheus.GaugeValue, float64(n))
	}
	if row, err := c.Stats.SumWalletBalances(ctx); err == nil {
		avail, _ := row.AvailableTotal.Float64()
		held, _ := row.HeldTotal.Float64()
		ch <- prometheus.MustNewConstMetric(walletAvailDesc, prometheus.GaugeValue, avail)
		ch <- prometheus.MustNewConstMetric(walletHeldDesc, prometheus.GaugeValue, held)
	}
	if n, err := c.Stats.CountOpenHolds(ctx); err == nil {
		ch <- prometheus.MustNewConstMetric(openHoldsDesc, prometheus.GaugeValue, float64(n))
	}
	if rows, err := c.Stats.CountLookupItemsByStatus(ctx); err == nil {
		for _, row := range rows {
			ch <- prometheus.MustNewConstMetric(lookupItemsDesc, prometheus.GaugeValue, float64(row.N), string(row.Status))
		}
	}
	if rows, err := c.Stats.CountLookupJobsByStatus(ctx); err == nil {
		for _, row := range rows {
			ch <- prometheus.MustNewConstMetric(lookupJobsDesc, prometheus.GaugeValue, float64(row.N), string(row.Status))
		}
	}
	if n, err := c.Stats.CountOpenLookupHolds(ctx); err == nil {
		ch <- prometheus.MustNewConstMetric(lookupHoldsDesc, prometheus.GaugeValue, float64(n))
	}
	lag := 0.0
	if at, err := c.Stats.OldestUnprocessedLookupCallbackAt(ctx); err == nil && at != nil {
		lag = time.Since(*at).Seconds()
		if lag < 0 {
			lag = 0
		}
	}
	ch <- prometheus.MustNewConstMetric(smscCallbackLagDesc, prometheus.GaugeValue, lag)
}

type LookupRuntimeCollector struct {
	Runtime LookupRuntime
}

func (c LookupRuntimeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- lookupEnabledDesc
	ch <- smscConfiguredDesc
	ch <- smscBalanceDesc
}

func (c LookupRuntimeCollector) Collect(ch chan<- prometheus.Metric) {
	if c.Runtime == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	enabled := 0.0
	if c.Runtime.LookupEnabled(ctx) {
		enabled = 1
	}
	configured := 0.0
	if c.Runtime.SMSCConfigured(ctx) {
		configured = 1
	}
	ch <- prometheus.MustNewConstMetric(lookupEnabledDesc, prometheus.GaugeValue, enabled)
	ch <- prometheus.MustNewConstMetric(smscConfiguredDesc, prometheus.GaugeValue, configured)
	if bal, ok := c.Runtime.SMSCBalance(ctx); ok {
		ch <- prometheus.MustNewConstMetric(smscBalanceDesc, prometheus.GaugeValue, bal)
	}
}

func ObservePDUMismatch(billed, provider *int32) {
	if billed == nil || provider == nil || *billed == *provider {
		return
	}
	PDUMismatch.Inc()
}

var (
	jobsDesc        = prometheus.NewDesc(ns+"_send_jobs", "Send jobs by status", []string{"status"}, nil)
	messagesDesc    = prometheus.NewDesc(ns+"_sms_messages", "SMS messages by status", []string{"status"}, nil)
	callbacksDesc   = prometheus.NewDesc(ns+"_callback_events_unprocessed", "Unprocessed provider callbacks", nil, nil)
	walletAvailDesc = prometheus.NewDesc(ns+"_wallet_available", "Sum of wallet available balances", nil, nil)
	walletHeldDesc  = prometheus.NewDesc(ns+"_wallet_held", "Sum of wallet held balances", nil, nil)
	openHoldsDesc       = prometheus.NewDesc(ns+"_billing_open_holds", "Open SMS HOLD ledger rows", nil, nil)
	lookupItemsDesc     = prometheus.NewDesc(ns+"_lookup_items", "Lookup items by status", []string{"status"}, nil)
	lookupJobsDesc      = prometheus.NewDesc(ns+"_lookup_jobs", "Lookup jobs by status", []string{"status"}, nil)
	lookupHoldsDesc     = prometheus.NewDesc(ns+"_lookup_holds_open", "Open lookup HOLD ledger rows", nil, nil)
	smscCallbackLagDesc = prometheus.NewDesc(ns+"_smsc_callback_lag_seconds", "Age of oldest unprocessed SMSC callback", nil, nil)
	lookupEnabledDesc   = prometheus.NewDesc(ns+"_lookup_enabled", "1 if lookup_enabled is on in Settings", nil, nil)
	smscConfiguredDesc  = prometheus.NewDesc(ns+"_smsc_configured", "1 if SMSC credentials are present in Settings", nil, nil)
	smscBalanceDesc     = prometheus.NewDesc(ns+"_smsc_balance", "SMSC cabinet balance from Redis cache", nil, nil)
)

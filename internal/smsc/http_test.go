package smsc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body any, status int) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func testHTTP(cfg Config, rt http.RoundTripper, log *slog.Logger) *HTTPClient {
	if cfg.Timeout == 0 {
		cfg.Timeout = time.Second
	}
	return NewHTTPClient(HTTPOptions{
		Config:     cfg,
		HTTPClient: &http.Client{Transport: rt},
		Sleep:      func(context.Context, time.Duration) error { return nil },
		Log:        log,
	})
}

func TestHTTPRetriesTransient503(t *testing.T) {
	var n atomic.Int32
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if n.Add(1) == 1 {
			return jsonResponse(map[string]any{"error": "busy"}, 503), nil
		}
		return jsonResponse(map[string]any{"id": 1, "cnt": 1}, 200), nil
	})
	cfg, err := Resolve(Input{Login: "u", Password: "p", RetryMaxAttempts: 2, RetryBaseDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	client := testHTTP(cfg, rt, slog.Default())
	result, err := client.Request(context.Background(), "/sys/send.php", map[string]any{"phones": "7999", "hlr": 1}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := result.Body.(map[string]any)
	if body["id"] != float64(1) || result.Attempts != 2 || n.Load() != 2 {
		t.Fatalf("body=%#v attempts=%d n=%d", body, result.Attempts, n.Load())
	}
}

func TestHTTPDoesNotRetry400(t *testing.T) {
	var n atomic.Int32
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		n.Add(1)
		return jsonResponse(map[string]any{"error": "bad"}, 400), nil
	})
	cfg, err := Resolve(Input{Login: "u", Password: "p", RetryMaxAttempts: 3, RetryBaseDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	client := testHTTP(cfg, rt, slog.Default())
	_, err = client.Request(context.Background(), "/sys/send.php", map[string]any{"phones": "7999"}, "", "")
	if AsError(err) == nil {
		t.Fatal("expected provider error")
	}
	if n.Load() != 1 {
		t.Fatalf("calls %d", n.Load())
	}
}

func TestHTTPTimeout(t *testing.T) {
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	cfg, err := Resolve(Input{Login: "u", Password: "p", Timeout: 5 * time.Millisecond, RetryMaxAttempts: 0})
	if err != nil {
		t.Fatal(err)
	}
	client := testHTTP(cfg, rt, slog.Default())
	_, err = client.Request(context.Background(), "/sys/balance.php", map[string]any{}, "", "")
	pe := AsError(err)
	if pe == nil || pe.Kind != KindTimeout || !pe.Retryable {
		t.Fatalf("got %#v", pe)
	}
}

type captureHandler struct {
	mu   sync.Mutex
	recs []map[string]any
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := map[string]any{"msg": r.Message}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.recs = append(h.recs, attrs)
	h.mu.Unlock()
	return nil
}
func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func TestHTTPRedactsSecretsInLogs(t *testing.T) {
	h := &captureHandler{}
	log := slog.New(h)
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(map[string]any{"balance": "1.00"}, 200), nil
	})
	cfg, err := Resolve(Input{Login: "secret-login", Password: "secret-pass"})
	if err != nil {
		t.Fatal(err)
	}
	client := testHTTP(cfg, rt, log)
	if _, err := client.Request(context.Background(), "/sys/balance.php", map[string]any{}, "", ""); err != nil {
		t.Fatal(err)
	}
	var params map[string]any
	h.mu.Lock()
	for _, rec := range h.recs {
		if rec["msg"] == "smsc.http.request" {
			params, _ = rec["params"].(map[string]any)
		}
	}
	h.mu.Unlock()
	if params["psw"] != "[REDACTED]" || params["login"] != "[REDACTED]" {
		t.Fatalf("params %#v", params)
	}
}

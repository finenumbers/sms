package smsc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var transientHTTP = map[int]struct{}{
	408: {},
	425: {},
	429: {},
	500: {},
	502: {},
	503: {},
	504: {},
}

type HTTPClient struct {
	cfg    Config
	source ConfigSource
	client *http.Client
	sleep  func(context.Context, time.Duration) error
	log    *slog.Logger
}

type HTTPOptions struct {
	Config     Config
	Source     ConfigSource
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
	Log        *slog.Logger
}

func NewHTTPClient(opts HTTPOptions) *HTTPClient {
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{}
	}
	sleep := opts.Sleep
	if sleep == nil {
		sleep = func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		}
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &HTTPClient{cfg: opts.Config, source: opts.Source, client: hc, sleep: sleep, log: log}
}

func (c *HTTPClient) current(ctx context.Context) (Config, error) {
	if c.source != nil {
		return c.source.SMSCConfig(ctx)
	}
	return c.cfg, nil
}

func (c *HTTPClient) Request(ctx context.Context, path string, params map[string]any, correlationID, kind string) (HTTPResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := c.current(ctx)
	if err != nil {
		return HTTPResult{}, err
	}
	merged := map[string]string{
		"fmt":     "3",
		"charset": "utf-8",
	}
	for k, v := range cfg.Auth.params() {
		merged[k] = v
	}
	for k, v := range params {
		if v == nil {
			continue
		}
		s := stringifyParam(v)
		if s == "" {
			continue
		}
		merged[k] = s
	}

	maxAttempts := cfg.RetryMaxAttempts + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		started := time.Now()
		form := url.Values{}
		for k, v := range merged {
			form.Set(k, v)
		}
		redacted, _ := RedactSecrets(valuesToMap(form)).(map[string]any)
		c.log.Debug("smsc.http.request",
			"path", path,
			"kind", kind,
			"attempt", attempt,
			"correlationId", correlationID,
			"params", redacted,
		)

		reqCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.endpoint(cfg, path), strings.NewReader(form.Encode()))
		if err != nil {
			cancel()
			return HTTPResult{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
		req.Header.Set("Accept", "application/json,text/plain,*/*")
		if correlationID != "" {
			req.Header.Set("X-Correlation-Id", correlationID)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			cancel()
			lastErr = err
			timeout := isTimeoutErr(reqCtx, err)
			retryable := timeout || isTransientNetworkError(err)
			if retryable && attempt < maxAttempts {
				c.log.Warn("smsc.http.retry",
					"path", path,
					"attempt", attempt,
					"timeout", timeout,
					"correlationId", correlationID,
					"message", err.Error(),
				)
				if sleepErr := c.sleep(ctx, cfg.RetryBaseDelay*time.Duration(attempt)); sleepErr != nil {
					return HTTPResult{}, sleepErr
				}
				continue
			}
			kind := KindNetwork
			msg := "SMSC network error: " + err.Error()
			if timeout {
				kind = KindTimeout
				msg = "SMSC request timed out after " + cfg.Timeout.String()
			}
			return HTTPResult{}, &Error{
				ProviderCode:  ProviderCode,
				Kind:          kind,
				Message:       msg,
				Retryable:     retryable,
				CorrelationID: correlationID,
				Err:           err,
			}
		}

		bodyBytes, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		cancel()
		if readErr != nil {
			lastErr = readErr
			if attempt < maxAttempts && isTransientNetworkError(readErr) {
				if sleepErr := c.sleep(ctx, cfg.RetryBaseDelay*time.Duration(attempt)); sleepErr != nil {
					return HTTPResult{}, sleepErr
				}
				continue
			}
			return HTTPResult{}, &Error{
				ProviderCode:  ProviderCode,
				Kind:          KindNetwork,
				Message:       "SMSC network error: " + readErr.Error(),
				Retryable:     isTransientNetworkError(readErr),
				CorrelationID: correlationID,
				Err:           readErr,
			}
		}

		parsed := parseJSONBody(string(bodyBytes))
		duration := time.Since(started)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if _, transient := transientHTTP[resp.StatusCode]; transient && attempt < maxAttempts {
				c.log.Warn("smsc.http.transient_http",
					"path", path,
					"httpStatus", resp.StatusCode,
					"attempt", attempt,
					"correlationId", correlationID,
				)
				if sleepErr := c.sleep(ctx, cfg.RetryBaseDelay*time.Duration(attempt)); sleepErr != nil {
					return HTTPResult{}, sleepErr
				}
				continue
			}
			kind := KindNetwork
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				kind = KindAuth
			}
			_, retryable := transientHTTP[resp.StatusCode]
			return HTTPResult{}, &Error{
				ProviderCode:  ProviderCode,
				Kind:          kind,
				Message:       "SMSC HTTP " + strconv.Itoa(resp.StatusCode),
				HTTPStatus:    resp.StatusCode,
				Retryable:     retryable,
				CorrelationID: correlationID,
				RawResponse:   parsed,
			}
		}

		c.log.Debug("smsc.http.response",
			"path", path,
			"kind", kind,
			"attempt", attempt,
			"httpStatus", resp.StatusCode,
			"durationMs", duration.Milliseconds(),
			"correlationId", correlationID,
			"body", RedactSecrets(parsed),
		)
		return HTTPResult{
			OK:         true,
			HTTPStatus: resp.StatusCode,
			Body:       parsed,
			Duration:   duration,
			Attempts:   attempt,
			URLPath:    path,
		}, nil
	}

	return HTTPResult{}, &Error{
		ProviderCode:  ProviderCode,
		Kind:          KindNetwork,
		Message:       "SMSC request failed after retries",
		Retryable:     true,
		CorrelationID: correlationID,
		Err:           lastErr,
	}
}

func (c *HTTPClient) endpoint(cfg Config, path string) string {
	base := strings.TrimRight(cfg.BaseURL, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func parseJSONBody(text string) any {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	var parsed any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err != nil {
		return map[string]any{"_nonJson": true, "text": trimmed}
	}
	return parsed
}

func valuesToMap(v url.Values) map[string]any {
	out := make(map[string]any, len(v))
	for k := range v {
		out[k] = v.Get(k)
	}
	return out
}

func stringifyParam(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return formatNumber(n)
	default:
		return asString(v)
	}
}

func isTimeoutErr(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}
	if ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func isTransientNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if isTimeoutErr(nil, err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "network")
}

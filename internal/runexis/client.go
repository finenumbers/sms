package runexis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"finenumbers/sms/internal/ops"
	"github.com/redis/go-redis/v9"
)

const (
	defaultBaseURL = "https://didapi.runexis.ru"
	tokenCacheKey  = "runexis:tokens"
	tokenLockKey   = "runexis:token_lock"
	lockTTL        = 8 * time.Second
	defaultSkew    = 60 * time.Second
)

var ErrNotConfigured = errors.New("runexis credentials not configured")

type Client struct {
	baseURL   string
	http      *http.Client
	rdb       redis.Cmdable
	creds     CredentialSource
	log       *slog.Logger
	ops       *ops.Logger
	now       func() time.Time
	skew      time.Duration
	refreshMu sync.Mutex
	memMu     sync.Mutex
	mem       *cachedTokens
}

type Options struct {
	BaseURL    string
	HTTPClient *http.Client
	Redis      redis.Cmdable
	Creds      CredentialSource
	Log        *slog.Logger
	Ops        *ops.Logger
	Now        func() time.Time
	Skew       time.Duration
}

type cachedTokens struct {
	Access     string    `json:"access"`
	Refresh    string    `json:"refresh"`
	AccessExp  time.Time `json:"access_exp"`
	RefreshExp time.Time `json:"refresh_exp"`
}

func New(opts Options) *Client {
	base := strings.TrimRight(opts.BaseURL, "/")
	if base == "" {
		base = defaultBaseURL
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	now := opts.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	skew := opts.Skew
	if skew <= 0 {
		skew = defaultSkew
	}
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	return &Client{
		baseURL: base,
		http:    hc,
		rdb:     opts.Redis,
		creds:   opts.Creds,
		log:     log,
		ops:     opts.Ops,
		now:     now,
		skew:    skew,
	}
}

func (c *Client) Invalidate(ctx context.Context) error {
	c.memMu.Lock()
	c.mem = nil
	c.memMu.Unlock()
	if c.rdb == nil {
		return nil
	}
	return c.rdb.Del(ctx, tokenCacheKey).Err()
}

func (c *Client) Login(ctx context.Context) (Tokens, error) {
	tok, err := c.login(ctx)
	if err != nil {
		return Tokens{}, err
	}
	if err := c.writeCache(ctx, tok); err != nil {
		return Tokens{}, err
	}
	return tok, nil
}

func (c *Client) Me(ctx context.Context) (Account, error) {
	var env wireEnvelope
	if _, err := c.doJSON(ctx, http.MethodGet, "/api/v1/me", nil, true, &env); err != nil {
		return Account{}, err
	}
	if !env.Success {
		return Account{}, envelopeError(0, env)
	}
	var d wireMeData
	if err := json.Unmarshal(env.Data, &d); err != nil {
		return Account{}, fmt.Errorf("me payload: %w", err)
	}
	return Account{Name: d.Name, Email: d.Email}, nil
}

func (c *Client) TestAuth(ctx context.Context) (Account, error) {
	_ = c.Invalidate(ctx)
	if _, err := c.token(ctx); err != nil {
		return Account{}, err
	}
	return c.Me(ctx)
}

func (c *Client) login(ctx context.Context) (Tokens, error) {
	if c.creds == nil {
		return Tokens{}, ErrNotConfigured
	}
	cr, err := c.creds.RunexisCredentials(ctx)
	if err != nil {
		return Tokens{}, err
	}
	if strings.TrimSpace(cr.Email) == "" || cr.Password == "" {
		return Tokens{}, ErrNotConfigured
	}
	var env wireEnvelope
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/login", wireLoginRequest{
		Email:    cr.Email,
		Password: cr.Password,
	}, false, &env); err != nil {
		return Tokens{}, err
	}
	if !env.Success {
		return Tokens{}, envelopeError(0, env)
	}
	return parseTokens(env.Data)
}

func (c *Client) refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	var env wireEnvelope
	if _, err := c.doJSON(ctx, http.MethodPost, "/api/v1/refresh", wireRefreshRequest{Token: refreshToken}, false, &env); err != nil {
		return Tokens{}, err
	}
	if !env.Success {
		return Tokens{}, envelopeError(0, env)
	}
	tok, err := parseTokens(env.Data)
	if err != nil {
		return Tokens{}, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return tok, nil
}

func (c *Client) token(ctx context.Context) (string, error) {
	now := c.now()
	if t, ok := c.readCache(ctx); ok && t.AccessExp.After(now.Add(c.skew)) {
		return t.Access, nil
	}
	unlock, err := c.acquireLock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	now = c.now()
	if t, ok := c.readCache(ctx); ok && t.AccessExp.After(now.Add(c.skew)) {
		return t.Access, nil
	}
	if t, ok := c.readCache(ctx); ok && t.Refresh != "" && t.RefreshExp.After(now) {
		nt, err := c.refresh(ctx, t.Refresh)
		if err == nil {
			_ = c.writeCache(ctx, nt)
			return nt.AccessToken, nil
		}
		c.log.Warn("runexis refresh failed, falling back to login")
	}
	nt, err := c.login(ctx)
	if err != nil {
		return "", err
	}
	if err := c.writeCache(ctx, nt); err != nil {
		return "", err
	}
	return nt.AccessToken, nil
}

func (c *Client) acquireLock(ctx context.Context) (func(), error) {
	if c.rdb == nil {
		c.refreshMu.Lock()
		return func() { c.refreshMu.Unlock() }, nil
	}
	for i := 0; i < 40; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ok, err := c.rdb.SetNX(ctx, tokenLockKey, "1", lockTTL).Result()
		if err == nil && ok {
			return func() { _ = c.rdb.Del(context.WithoutCancel(ctx), tokenLockKey).Err() }, nil
		}
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	c.refreshMu.Lock()
	return func() { c.refreshMu.Unlock() }, nil
}

func (c *Client) readCache(ctx context.Context) (cachedTokens, bool) {
	if c.rdb != nil {
		s, err := c.rdb.Get(ctx, tokenCacheKey).Result()
		if err == nil && s != "" {
			var t cachedTokens
			if json.Unmarshal([]byte(s), &t) == nil && t.Access != "" {
				return t, true
			}
		}
	}
	c.memMu.Lock()
	defer c.memMu.Unlock()
	if c.mem == nil {
		return cachedTokens{}, false
	}
	return *c.mem, true
}

func (c *Client) writeCache(ctx context.Context, tok Tokens) error {
	now := c.now()
	ct := cachedTokens{
		Access:     tok.AccessToken,
		Refresh:    tok.RefreshToken,
		AccessExp:  tok.AccessExpiresAt,
		RefreshExp: tok.RefreshExpiresAt,
	}
	c.memMu.Lock()
	if tok.AccessExpiresAt.After(now) {
		c.mem = &ct
	} else {
		c.mem = nil
	}
	c.memMu.Unlock()
	if c.rdb == nil {
		return nil
	}
	ttl := tok.RefreshExpiresAt.Sub(now)
	if ttl <= 0 {
		_ = c.rdb.Del(ctx, tokenCacheKey).Err()
		return nil
	}
	b, err := json.Marshal(ct)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, tokenCacheKey, b, ttl).Err()
}

func (c *Client) doJSON(ctx context.Context, method, path string, reqBody any, auth bool, env *wireEnvelope) ([]byte, error) {
	return c.doJSONAuth(ctx, method, path, reqBody, auth, env, true)
}

func (c *Client) doJSONAuth(ctx context.Context, method, path string, reqBody any, auth bool, env *wireEnvelope, retry401 bool) ([]byte, error) {
	started := c.now()
	var reqJSON []byte
	var rdr io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			c.recordOps(ctx, method, path, nil, nil, 0, started, err)
			return nil, err
		}
		reqJSON = append([]byte(nil), b...)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		c.recordOps(ctx, method, path, reqJSON, nil, 0, started, err)
		return nil, err
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if auth {
		tok, err := c.token(ctx)
		if err != nil {
			c.recordOps(ctx, method, path, reqJSON, nil, 0, started, err)
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	start := c.now()
	resp, err := c.http.Do(req)
	if err != nil {
		c.recordOps(ctx, method, path, reqJSON, nil, 0, start, err)
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		c.recordOps(ctx, method, path, reqJSON, nil, resp.StatusCode, start, err)
		return nil, err
	}
	c.log.Info("runexis http",
		"method", method,
		"path", path,
		"status", resp.StatusCode,
		"latency_ms", c.now().Sub(start).Milliseconds(),
	)

	if auth && resp.StatusCode == http.StatusUnauthorized && retry401 {
		c.recordOps(ctx, method, path, reqJSON, body, resp.StatusCode, start, nil)
		_ = c.Invalidate(ctx)
		return c.doJSONAuth(ctx, method, path, reqBody, true, env, false)
	}

	if err := json.Unmarshal(body, env); err != nil {
		if resp.StatusCode >= 400 {
			err = &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
			c.recordOps(ctx, method, path, reqJSON, body, resp.StatusCode, start, err)
			return body, err
		}
		err = fmt.Errorf("runexis decode: %w", err)
		c.recordOps(ctx, method, path, reqJSON, body, resp.StatusCode, start, err)
		return body, err
	}
	if resp.StatusCode >= 400 {
		err := envelopeError(resp.StatusCode, *env)
		c.recordOps(ctx, method, path, reqJSON, body, resp.StatusCode, start, err)
		return body, err
	}
	c.recordOps(ctx, method, path, reqJSON, body, resp.StatusCode, start, nil)
	return body, nil
}

func skipReconcileStatisticOps(ctx context.Context, method, path string, status int, callErr error) bool {
	if callErr != nil || status == 0 || status >= 400 {
		return false
	}
	if method != http.MethodGet || path != "/api/v1/sms/statistic" {
		return false
	}
	f := ops.From(ctx)
	return f != nil && f.RequestID == "reconcile"
}

func (c *Client) recordOps(ctx context.Context, method, path string, reqJSON, respBody []byte, status int, start time.Time, callErr error) {
	if c == nil || c.ops == nil {
		return
	}
	if skipReconcileStatisticOps(ctx, method, path, status, callErr) {
		return
	}
	latency := int32(c.now().Sub(start).Milliseconds())
	if latency < 0 {
		latency = 0
	}
	level := ops.LevelInfo
	switch {
	case callErr != nil && (status == 0 || status >= 500):
		level = ops.LevelError
	case status >= 500:
		level = ops.LevelError
	case status >= 400 || callErr != nil:
		level = ops.LevelWarn
	}
	summary := method + " " + path
	if status > 0 {
		summary = fmt.Sprintf("%s %s %d", method, path, status)
	}
	ev := ops.Event{
		Category:   ops.CategoryDIDAPI,
		Level:      level,
		Action:     path,
		HTTPMethod: method,
		HTTPPath:   path,
		HTTPStatus: status,
		LatencyMS:  latency,
		Summary:    summary,
	}
	if callErr != nil {
		ev.Error = callErr.Error()
	}
	detail := map[string]any{
		"http_status": status,
		"latency_ms":  latency,
	}
	if strings.Contains(path, "/sms/send") {
		detail["success"] = callErr == nil && status > 0 && status < 400
		smsID := ""
		if len(respBody) > 0 {
			if env, err := decodeEnvelope(respBody); err == nil {
				smsID = extractSMSID(env.Data)
			}
		}
		detail["sms_id"] = smsID
	}
	if len(reqJSON) > 0 {
		detail["request"] = ops.RawJSON(reqJSON)
	}
	if len(respBody) > 0 {
		detail["response"] = ops.RawJSON(respBody)
	}
	ev.Detail = detail
	c.ops.Write(ctx, ev)
}

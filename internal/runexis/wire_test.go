package runexis

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	p := filepath.Join("..", "..", "docs", "reference", "runexis", "fixtures", name)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSpace(b)
}

func TestMarshalSendToNumberIsJSONString(t *testing.T) {
	raw, err := marshalSend(SendInput{From: "79991112233", To: "79993332211", Text: "Пример сообщения"})
	if err != nil {
		t.Fatal(err)
	}
	want := fixture(t, "sms_send_request.json")
	var gotObj, wantObj any
	if err := json.Unmarshal(raw, &gotObj); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(want, &wantObj); err != nil {
		t.Fatal(err)
	}
	gotNorm, _ := json.Marshal(gotObj)
	wantNorm, _ := json.Marshal(wantObj)
	if string(gotNorm) != string(wantNorm) {
		t.Fatalf("got %s want %s", gotNorm, wantNorm)
	}
	if !bytes.Contains(raw, []byte(`"to_number":"79993332211"`)) {
		t.Fatalf("to_number not string: %s", raw)
	}
	if bytes.Contains(raw, []byte(`"to_number":79993332211`)) || bytes.Contains(raw, []byte(".0")) || bytes.Contains(raw, []byte("e+")) {
		t.Fatalf("to_number not JSON string: %s", raw)
	}
}

// Support 2026-08-14: to_number must be a JSON string. Integer body returned HTTP 500.
func TestMarshalSendToNumberStringContract(t *testing.T) {
	raw, err := marshalSend(SendInput{From: "79391125968", To: "79994504444", Text: "test"})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"from_number":"79391125968","to_number":"79994504444","text":"test"}`
	if string(raw) != want {
		t.Fatalf("got %s want %s", raw, want)
	}
}

func TestParseSendResponseProvisionalAndEmpty(t *testing.T) {
	got, err := parseSendResponse(fixture(t, "sms_send_response.provisional.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderSMSID != "8ace264b-a78e-488e-b411-af2581bd3f23" {
		t.Fatalf("sms_id=%q", got.ProviderSMSID)
	}
	empty, err := parseSendResponse(fixture(t, "sms_send_response.empty.json"))
	if err != nil {
		t.Fatal(err)
	}
	if empty.ProviderSMSID != "" {
		t.Fatalf("empty data should have no sms_id, got %q", empty.ProviderSMSID)
	}
}

func TestParseLoginFixture(t *testing.T) {
	env, err := decodeEnvelope(fixture(t, "auth_login_response.json"))
	if err != nil {
		t.Fatal(err)
	}
	tok, err := parseTokens(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		t.Fatal("missing tokens")
	}
	if tok.AccessExpiresAt.IsZero() || tok.RefreshExpiresAt.IsZero() {
		t.Fatal("missing expiry")
	}
}

func TestAPIErrorIncludesRequestID(t *testing.T) {
	err := &APIError{Status: 500, Message: "an unexpected error has occurred", RequestID: "req-1"}
	if err.Error() != "runexis: an unexpected error has occurred (request_id=req-1)" {
		t.Fatalf("%q", err.Error())
	}
	if (&APIError{Status: 400, Message: "account not found"}).Error() != "runexis: account not found" {
		t.Fatal("no request_id")
	}
}

type staticCreds struct {
	email, password string
}

func (s staticCreds) RunexisCredentials(context.Context) (Credentials, error) {
	return Credentials{Email: s.email, Password: s.password}, nil
}

func TestClientSmokeAgainstFixtures(t *testing.T) {
	loginBody := fixture(t, "auth_login_response.json")
	refreshBody := fixture(t, "auth_refresh_response.json")
	meBody := fixture(t, "auth_me_response.json")
	sendResp := fixture(t, "sms_send_response.provisional.json")
	statResp := fixture(t, "sms_statistic_response.json")

	var sawStatisticBody bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/login":
			var req wireLoginRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
				t.Errorf("login body: %+v %v", req, err)
			}
			w.Write(loginBody)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/refresh":
			w.Write(refreshBody)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/me":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Write(meBody)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/sms/send":
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type=%q", r.Header.Get("Content-Type"))
			}
			if r.Header.Get("Accept") != "application/json" {
				t.Errorf("Accept=%q", r.Header.Get("Accept"))
			}
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				t.Error("send missing Bearer")
			}
			raw, _ := io.ReadAll(r.Body)
			if !bytes.Contains(raw, []byte(`"to_number":"79993332211"`)) {
				t.Errorf("send wire: %s", raw)
			}
			var wire wireSendRequest
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Errorf("send json: %v", err)
			}
			w.Write(sendResp)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/sms/statistic":
			raw, _ := io.ReadAll(r.Body)
			if len(bytes.TrimSpace(raw)) == 0 {
				t.Error("statistic GET missing JSON body")
			}
			var req wireStatisticRequest
			if err := json.Unmarshal(raw, &req); err != nil {
				t.Errorf("statistic body: %s", raw)
			}
			if strings.Contains(string(raw), "[[") {
				t.Errorf("nested sender_numbers: %s", raw)
			}
			sawStatisticBody = true
			w.Write(statResp)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	c := New(Options{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Redis:      rdb,
		Creds:      staticCreds{email: "ivan.ivanov@example.com", password: "secret"},
		Now:        func() time.Time { return time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC) },
	})

	acc, err := c.TestAuth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if acc.Email != "user@example.com" {
		t.Fatalf("me email=%q", acc.Email)
	}

	out, err := c.Send(context.Background(), SendInput{From: "79991112233", To: "79993332211", Text: "Пример сообщения"})
	if err != nil {
		t.Fatal(err)
	}
	if out.ProviderSMSID != "8ace264b-a78e-488e-b411-af2581bd3f23" {
		t.Fatalf("sms_id=%q", out.ProviderSMSID)
	}

	page, err := c.Statistic(context.Background(), StatisticQuery{
		From:  time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		To:    time.Date(2025, 12, 31, 20, 59, 59, 0, time.UTC),
		Page:  1,
		Limit: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawStatisticBody {
		t.Fatal("statistic body not observed")
	}
	if page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("statistic page=%+v", page)
	}

	cached, err := rdb.Get(context.Background(), tokenCacheKey).Result()
	if err != nil || cached == "" {
		t.Fatalf("token cache: %v %q", err, cached)
	}
	if strings.Contains(cached, "secret") {
		t.Fatal("password leaked into redis cache")
	}
}

func TestRefreshPersistsNewRefreshToken(t *testing.T) {
	loginN := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/login":
			loginN++
			w.Write([]byte(`{"success":true,"data":{"token":"access-old","refresh_token":"refresh-old","token_expire":"2025-07-01T00:00:30.000Z","refresh_token_expire":"2025-08-01T00:00:00.000Z"}}`))
		case "/api/v1/refresh":
			var req wireRefreshRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Token != "refresh-old" {
				t.Errorf("refresh token=%q", req.Token)
			}
			w.Write([]byte(`{"success":true,"data":{"token":"access-new","refresh_token":"refresh-new","token_expire":"2025-07-01T01:00:00.000Z","refresh_token_expire":"2025-08-01T00:00:00.000Z"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	now := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	c := New(Options{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Creds:      staticCreds{email: "a@b.c", password: "x"},
		Now:        func() time.Time { return now },
		Skew:       time.Minute,
	})
	if _, err := c.token(context.Background()); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second) // still within access TTL but inside skew (expire at +30s, skew 1m)
	tok, err := c.token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "access-new" {
		t.Fatalf("token=%q", tok)
	}
	if loginN != 1 {
		t.Fatalf("login count=%d", loginN)
	}
	if c.mem == nil || c.mem.Refresh != "refresh-new" {
		t.Fatalf("refresh not saved: %+v", c.mem)
	}
}

func TestFormatMoscow(t *testing.T) {
	got := formatMoscow(time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC))
	if got != "2025-12-01 03:00:00" {
		t.Fatalf("got %q", got)
	}
}

func TestMarshalStatisticFlatAndMoscow(t *testing.T) {
	raw, err := marshalStatistic(StatisticQuery{
		From:          time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
		To:            time.Date(2025, 12, 31, 20, 59, 59, 0, time.UTC),
		SenderNumbers: []string{"79996665522"},
		Page:          1,
		Limit:         30,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("[[")) {
		t.Fatalf("nested arrays: %s", raw)
	}
	if !bytes.Contains(raw, []byte(`"from":"2025-12-01 03:00:00"`)) {
		t.Fatalf("moscow from missing: %s", raw)
	}
}

func TestTokenCancelledContextDoesNotLogin(t *testing.T) {
	loginN := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/login" {
			loginN++
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	_ = rdb.Set(context.Background(), tokenLockKey, "1", time.Minute).Err()

	c := New(Options{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Redis:      rdb,
		Creds:      staticCreds{email: "a@b.c", password: "x"},
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.token(ctx); err == nil {
		t.Fatal("expected error")
	}
	if loginN != 0 {
		t.Fatalf("login count=%d", loginN)
	}
}

func TestWriteCacheSkipsExpiredRefresh(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	now := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	c := New(Options{
		Redis: rdb,
		Now:   func() time.Time { return now },
	})
	err := c.writeCache(context.Background(), Tokens{
		AccessToken:      "a",
		RefreshToken:     "r",
		AccessExpiresAt:  now.Add(-time.Minute),
		RefreshExpiresAt: now.Add(-time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if mr.Exists(tokenCacheKey) {
		t.Fatal("expired tokens cached in redis")
	}
}

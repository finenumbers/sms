package runexis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseManagedNumbersFixture(t *testing.T) {
	env, err := decodeEnvelope(fixture(t, "numbers_management_response.json"))
	if err != nil {
		t.Fatal(err)
	}
	page, err := parseManagedNumbersPage(env, 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Page != 1 || page.Limit != 30 || len(page.Items) != 1 {
		t.Fatalf("page=%+v", page)
	}
	n := page.Items[0]
	if n.Code != "999" || n.Number != "1234567" || n.CityName != "Moscow" {
		t.Fatalf("item=%+v", n)
	}
	var snap map[string]any
	if err := json.Unmarshal(n.Snapshot, &snap); err != nil {
		t.Fatal(err)
	}
	if _, ok := snap["comment"]; ok {
		t.Fatal("comment leaked into snapshot")
	}
	if _, ok := snap["partner"]; ok {
		t.Fatal("partner leaked into snapshot")
	}
	if _, ok := snap["responsible"]; ok {
		t.Fatal("responsible leaked into snapshot")
	}
	for _, k := range []string{"id", "status", "city", "tariff", "class", "operator"} {
		if _, ok := snap[k]; !ok {
			t.Fatalf("snapshot missing %s: %s", k, n.Snapshot)
		}
	}
	if _, ok := snap["status"].(map[string]any); !ok {
		t.Fatalf("status not object: %T %s", snap["status"], n.Snapshot)
	}
}

func TestListManagedNumbersAndSMSAccount(t *testing.T) {
	loginBody := fixture(t, "auth_login_response.json")
	mgmtBody := fixture(t, "numbers_management_response.json")
	acctBody := fixture(t, "sms_account_response.json")
	errBody := fixture(t, "error_400.json")

	var sawManagementBody bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/login":
			w.Write(loginBody)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/numbers/management":
			raw, _ := io.ReadAll(r.Body)
			if len(bytes.TrimSpace(raw)) > 0 {
				t.Errorf("management GET must not have a JSON body: %s", raw)
				sawManagementBody = true
			}
			if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("limit") != "30" {
				t.Errorf("query=%s", r.URL.RawQuery)
			}
			if r.URL.Query().Get("number_status_id") != "" {
				t.Error("must not filter number_status_id")
			}
			w.Write(mgmtBody)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/numbers/79991234567/sms/account":
			w.Write(acctBody)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sms/account"):
			w.WriteHeader(http.StatusNotFound)
			w.Write(errBody)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	c := New(Options{
		BaseURL:    srv.URL,
		HTTPClient: srv.Client(),
		Creds:      staticCreds{email: "ivan.ivanov@example.com", password: "secret"},
		Now:        func() time.Time { return time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC) },
	})

	page, err := c.ListManagedNumbers(context.Background(), 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	if sawManagementBody {
		t.Fatal("management request had a body")
	}
	if len(page.Items) != 1 || page.Items[0].Code != "999" {
		t.Fatalf("page=%+v", page)
	}

	acct, err := c.SMSAccount(context.Background(), "79991234567")
	if err != nil {
		t.Fatal(err)
	}
	if !acct.In || !acct.DomOut || acct.IntOut || !acct.InMass {
		t.Fatalf("account=%+v", acct)
	}

	_, err = c.SMSAccount(context.Background(), "79990000000")
	if !IsNoSMS(err) {
		t.Fatalf("want ErrNoSMS, got %v", err)
	}
	if !errors.Is(err, ErrNoSMS) {
		t.Fatalf("errors.Is ErrNoSMS: %v", err)
	}
}

func TestIsNoSMS(t *testing.T) {
	if !IsNoSMS(&APIError{Status: 404}) {
		t.Fatal("404")
	}
	if !IsNoSMS(&APIError{Status: 400}) {
		t.Fatal("400")
	}
	if IsNoSMS(&APIError{Status: 401}) {
		t.Fatal("401 is auth, not no-SMS")
	}
	if IsNoSMS(&APIError{Status: 500}) {
		t.Fatal("500")
	}
	if IsNoSMS(errors.New("net")) {
		t.Fatal("other")
	}
}

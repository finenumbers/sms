package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"finenumbers/sms/internal/config"
)

func surface(cfg config.Config) http.Handler {
	return RestrictSurface(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func hit(t *testing.T, h http.Handler, method, path, host string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestRestrictSurfaceUnknownHostDeniesAPIWhenHostsSet(t *testing.T) {
	h := surface(config.Config{
		AdminHost:  "admin.sms.localhost",
		ClientHost: "client.sms.localhost",
		APIHost:    "api.sms.localhost",
	})
	if code := hit(t, h, http.MethodGet, "/admin/v1/health", "127.0.0.1:8080"); code != http.StatusNotFound {
		t.Fatalf("unknown host API status=%d", code)
	}
	if code := hit(t, h, http.MethodGet, "/clients", "finenumbers-api:8080"); code != http.StatusNotFound {
		t.Fatalf("unknown host SPA status=%d", code)
	}
	if code := hit(t, h, http.MethodGet, "/healthz", "127.0.0.1:8080"); code != http.StatusOK {
		t.Fatalf("unknown host health status=%d", code)
	}
	if code := hit(t, h, http.MethodGet, "/metrics", "api:8080"); code != http.StatusOK {
		t.Fatalf("internal scrape status=%d", code)
	}
}

func TestRestrictSurfacePassThroughWhenHostsUnset(t *testing.T) {
	h := surface(config.Config{})
	if code := hit(t, h, http.MethodGet, "/admin/v1/health", "127.0.0.1:8080"); code != http.StatusOK {
		t.Fatalf("pass-through status=%d", code)
	}
}

func TestRestrictSurfaceRejectsAdminOnAPIHost(t *testing.T) {
	h := surface(config.Config{
		AdminHost:  "admin.sms.localhost",
		ClientHost: "client.sms.localhost",
		APIHost:    "api.sms.localhost",
	})
	if code := hit(t, h, http.MethodGet, "/admin/v1/health", "api.sms.localhost"); code != http.StatusNotFound {
		t.Fatalf("status=%d", code)
	}
}

func TestRestrictSurfaceAllowsHealthOnAPIHost(t *testing.T) {
	h := surface(config.Config{APIHost: "api.sms.localhost"})
	if code := hit(t, h, http.MethodGet, "/healthz", "api.sms.localhost"); code != http.StatusOK {
		t.Fatalf("status=%d", code)
	}
}

func TestRestrictSurfaceAllowsAdminOnAdminHost(t *testing.T) {
	h := surface(config.Config{
		AdminHost: "admin.sms.localhost",
		APIHost:   "api.sms.localhost",
	})
	if code := hit(t, h, http.MethodGet, "/admin/v1/settings", "admin.sms.localhost"); code != http.StatusOK {
		t.Fatalf("status=%d", code)
	}
}

func TestRestrictSurfaceSPAOnAdminAndClient(t *testing.T) {
	h := surface(config.Config{
		AdminHost:  "admin.sms.localhost",
		ClientHost: "client.sms.localhost",
		APIHost:    "api.sms.localhost",
	})
	if code := hit(t, h, http.MethodGet, "/clients", "admin.sms.localhost"); code != http.StatusOK {
		t.Fatalf("admin SPA GET status=%d", code)
	}
	if code := hit(t, h, http.MethodGet, "/assets/index.js", "admin.sms.localhost"); code != http.StatusOK {
		t.Fatalf("admin assets status=%d", code)
	}
	if code := hit(t, h, http.MethodPost, "/clients", "admin.sms.localhost"); code != http.StatusNotFound {
		t.Fatalf("admin SPA POST status=%d", code)
	}
	if code := hit(t, h, http.MethodGet, "/inbox", "client.sms.localhost"); code != http.StatusOK {
		t.Fatalf("client SPA GET status=%d", code)
	}
	if code := hit(t, h, http.MethodGet, "/", "api.sms.localhost"); code != http.StatusNotFound {
		t.Fatalf("api SPA status=%d", code)
	}
}

func TestRestrictSurfaceRejectsForeignPrefixes(t *testing.T) {
	h := surface(config.Config{
		AdminHost:  "admin.sms.localhost",
		ClientHost: "client.sms.localhost",
		APIHost:    "api.sms.localhost",
	})
	cases := []struct{ method, path, host string }{
		{http.MethodGet, "/client/v1/health", "admin.sms.localhost"},
		{http.MethodGet, "/v1/health", "admin.sms.localhost"},
		{http.MethodPost, "/internal/runexis/dlr/tok", "admin.sms.localhost"},
		{http.MethodPost, "/internal/smsc/callback", "admin.sms.localhost"},
		{http.MethodGet, "/internal/smsc/callback", "client.sms.localhost"},
		{http.MethodGet, "/admin/v1/health", "client.sms.localhost"},
		{http.MethodGet, "/v1/health", "client.sms.localhost"},
		{http.MethodGet, "/admin/v1/health", "api.sms.localhost"},
		{http.MethodGet, "/client/v1/health", "api.sms.localhost"},
	}
	for _, tc := range cases {
		if code := hit(t, h, tc.method, tc.path, tc.host); code != http.StatusNotFound {
			t.Fatalf("%s %s host=%s status=%d", tc.method, tc.path, tc.host, code)
		}
	}
}

func TestRestrictSurfaceRejectsMetricsOnPublicHosts(t *testing.T) {
	h := surface(config.Config{
		AdminHost:  "admin.sms.localhost",
		ClientHost: "client.sms.localhost",
		APIHost:    "api.sms.localhost",
	})
	for _, host := range []string{"admin.sms.localhost", "client.sms.localhost", "api.sms.localhost"} {
		if code := hit(t, h, http.MethodGet, "/metrics", host); code != http.StatusNotFound {
			t.Fatalf("host=%s status=%d", host, code)
		}
	}
}

func TestRestrictSurfaceAllowsIngressOnAPIHost(t *testing.T) {
	h := surface(config.Config{APIHost: "api.sms.localhost"})
	if code := hit(t, h, http.MethodPost, "/internal/runexis/dlr/tok", "api.sms.localhost"); code != http.StatusOK {
		t.Fatalf("status=%d", code)
	}
	if code := hit(t, h, http.MethodPost, "/internal/smsc/callback", "api.sms.localhost"); code != http.StatusOK {
		t.Fatalf("smsc callback status=%d", code)
	}
}

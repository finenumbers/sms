package httpserver

import (
	"net/http"
	"testing"
)

func TestShouldLogHTTP(t *testing.T) {
	cases := []struct {
		method, path string
		status       int
		want         bool
	}{
		{http.MethodGet, "/admin/v1/clients", 200, false},
		{http.MethodGet, "/admin/v1/clients", 304, false},
		{http.MethodGet, "/admin/v1/clients", 404, false},
		{http.MethodGet, "/admin/v1/clients", 403, true},
		{http.MethodGet, "/admin/v1/auth/me", 401, false},
		{http.MethodGet, "/client/v1/auth/me", 401, false},
		{http.MethodGet, "/admin/v1/logs", 200, false},
		{http.MethodGet, "/admin/v1/logs/abc", 500, false},
		{http.MethodGet, "/healthz", 200, false},
		{http.MethodGet, "/assets/app.js", 200, false},
		{http.MethodOptions, "/admin/v1/clients", 204, false},
		{http.MethodPost, "/admin/v1/clients", 201, true},
		{http.MethodPatch, "/admin/v1/settings", 200, true},
		{http.MethodPost, "/internal/runexis/dlr/tok", 200, false},
		{http.MethodPost, "/internal/runexis/dlr/tok", 404, true},
		{http.MethodPost, "/internal/runexis/dlr/tok", 413, true},
		{http.MethodGet, "/internal/runexis/dlr/tok", 404, false},
		{http.MethodPost, "/internal/smsc/callback", 200, false},
		{http.MethodPost, "/internal/smsc/callback", 401, true},
	}
	for _, tc := range cases {
		got := shouldLogHTTP(tc.method, tc.path, tc.status)
		if got != tc.want {
			t.Errorf("%s %s %d: got %v want %v", tc.method, tc.path, tc.status, got, tc.want)
		}
	}
}

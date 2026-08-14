package httpserver

import (
	"net/http"
	"strings"

	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/httpx"
)

func RestrictSurface(cfg config.Config) func(http.Handler) http.Handler {
	admin := httpx.HostOnly(cfg.AdminHost)
	client := httpx.HostOnly(cfg.ClientHost)
	api := httpx.HostOnly(cfg.APIHost)
	hostsSet := admin != "" || client != "" || api != ""
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := httpx.HostOnly(r.Host)
			path := r.URL.Path
			var allowed bool
			switch {
			case admin != "" && host == admin:
				allowed = adminSurfaceAllowed(r.Method, path)
			case client != "" && host == client:
				allowed = clientSurfaceAllowed(r.Method, path)
			case api != "" && host == api:
				allowed = apiSurfaceAllowed(path)
			default:
				if !hostsSet {
					next.ServeHTTP(w, r)
					return
				}
				allowed = isHealth(path) || isMetrics(path)
			}
			if !allowed {
				httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func adminSurfaceAllowed(method, path string) bool {
	if isHealth(path) {
		return true
	}
	if isMetrics(path) || foreignToAdmin(path) {
		return false
	}
	if hasPrefix(path, "/admin/v1") {
		return true
	}
	return spaMethod(method)
}

func clientSurfaceAllowed(method, path string) bool {
	if isHealth(path) {
		return true
	}
	if isMetrics(path) || foreignToClient(path) {
		return false
	}
	if hasPrefix(path, "/client/v1") {
		return true
	}
	return spaMethod(method)
}

func apiSurfaceAllowed(path string) bool {
	if isHealth(path) {
		return true
	}
	if isMetrics(path) {
		return false
	}
	return hasPrefix(path, "/v1") || hasPrefix(path, "/internal/runexis") || hasPrefix(path, "/internal/smsc")
}

func foreignToAdmin(path string) bool {
	return hasPrefix(path, "/client/v1") || hasPrefix(path, "/v1") || hasPrefix(path, "/internal/runexis") || hasPrefix(path, "/internal/smsc")
}

func foreignToClient(path string) bool {
	return hasPrefix(path, "/admin/v1") || hasPrefix(path, "/v1") || hasPrefix(path, "/internal/runexis") || hasPrefix(path, "/internal/smsc")
}

func spaMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func isHealth(path string) bool {
	return path == "/healthz" || path == "/readyz"
}

func isMetrics(path string) bool {
	return path == "/metrics" || strings.HasPrefix(path, "/metrics/")
}

func hasPrefix(path, prefix string) bool {
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

package httpserver

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/ingress"
	"finenumbers/sms/internal/metrics"
	"finenumbers/sms/internal/ops"
)

func requestLogger(log *slog.Logger, opsLog *ops.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			r = r.WithContext(ops.Attach(r.Context()))
			if id := middleware.GetReqID(r.Context()); id != "" {
				r = r.WithContext(ops.ContextWith(r.Context(), ops.Fields{RequestID: id}))
			}
			next.ServeHTTP(ww, r)
			status := ww.Status()
			path := ingress.RedactPath(r.URL.Path)
			dur := time.Since(start)
			metrics.ObserveHTTP(path, r.Method, status, dur)
			if log != nil {
				log.Info("http",
					"method", r.Method,
					"path", path,
					"status", status,
					"bytes", ww.BytesWritten(),
					"duration_ms", dur.Milliseconds(),
					"request_id", middleware.GetReqID(r.Context()),
				)
			}
			if opsLog != nil && shouldLogHTTP(r.Method, r.URL.Path, status) {
				level := ops.LevelInfo
				if status >= 500 {
					level = ops.LevelError
				} else if status >= 400 {
					level = ops.LevelWarn
				}
				opsLog.Write(r.Context(), ops.Event{
					Category:   ops.CategoryHTTP,
					Level:      level,
					Action:     "http.request",
					HTTPMethod: r.Method,
					HTTPPath:   path,
					HTTPStatus: status,
					LatencyMS:  int32(dur.Milliseconds()),
					Summary:    fmt.Sprintf("%s %s %d", r.Method, path, status),
					IP:         httpx.ClientIP(r),
				})
			}
		})
	}
}

func shouldLogHTTP(method, path string, status int) bool {
	if method == http.MethodOptions {
		return false
	}
	switch {
	case path == "/healthz" || path == "/readyz" || path == "/metrics" || strings.HasPrefix(path, "/metrics/"):
		return false
	case strings.HasPrefix(path, "/assets/"):
		return false
	case path == "/admin/v1/logs" || strings.HasPrefix(path, "/admin/v1/logs/"):
		return false
	}
	if isIngressPath(path) && status >= 200 && status < 300 {
		return false
	}
	switch method {
	case http.MethodGet, http.MethodHead:
		if status == http.StatusNotFound {
			return false
		}
		if status == http.StatusUnauthorized && strings.HasSuffix(path, "/auth/me") {
			return false
		}
		if status >= 200 && status < 400 {
			return false
		}
		return status >= 400
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func isIngressPath(path string) bool {
	return path == "/internal/runexis" || strings.HasPrefix(path, "/internal/runexis/") ||
		path == "/internal/smsc" || strings.HasPrefix(path, "/internal/smsc/")
}

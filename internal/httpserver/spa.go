package httpserver

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"finenumbers/sms/internal/config"
	"finenumbers/sms/internal/httpx"
)

func SPA(cfg config.Config) http.Handler {
	admin := httpx.HostOnly(cfg.AdminHost)
	client := httpx.HostOnly(cfg.ClientHost)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		host := httpx.HostOnly(r.Host)
		var root string
		switch {
		case admin != "" && host == admin:
			root = cfg.AdminSPADir
		case client != "" && host == client:
			root = cfg.ClientSPADir
		default:
			httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if strings.TrimSpace(root) == "" {
			httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		serveSPA(w, r, root)
	})
}

func serveSPA(w http.ResponseWriter, r *http.Request, root string) {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	reqPath := pathClean(r.URL.Path)
	full, ok := underRoot(root, reqPath)
	if !ok {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "same-origin")
	if fi, err := os.Stat(full); err == nil && !fi.IsDir() {
		if strings.HasPrefix(reqPath, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, full)
		return
	}
	index := filepath.Join(root, "index.html")
	if _, err := os.Stat(index); err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, index)
}

func pathClean(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	return path.Clean("/" + strings.TrimPrefix(p, "/"))
}

func underRoot(root, urlPath string) (string, bool) {
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rel := strings.TrimPrefix(urlPath, "/")
	full := filepath.Join(cleanRoot, filepath.FromSlash(rel))
	full, err = filepath.Abs(full)
	if err != nil {
		return "", false
	}
	sep := string(os.PathSeparator)
	if full != cleanRoot && !strings.HasPrefix(full, cleanRoot+sep) {
		return "", false
	}
	return full, true
}

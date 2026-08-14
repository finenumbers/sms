package httpserver

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"finenumbers/sms/internal/config"
)

func TestSPAServesIndexAndAssets(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<html>admin</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := SPA(config.Config{AdminHost: "admin.sms.localhost", AdminSPADir: root})

	req := httptest.NewRequest(http.MethodGet, "/clients", nil)
	req.Host = "admin.sms.localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "admin") {
		t.Fatalf("index status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("index cache=%q", rec.Header().Get("Cache-Control"))
	}

	req = httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Host = "admin.sms.localhost"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("asset status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset cache=%q", rec.Header().Get("Cache-Control"))
	}
}

func TestSPARejectsWrongHost(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := SPA(config.Config{AdminHost: "admin.sms.localhost", AdminSPADir: root})

	req := httptest.NewRequest(http.MethodGet, "/clients", nil)
	req.Host = "api.sms.localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("wrong host status=%d", rec.Code)
	}
}

func TestUnderRootStaysInside(t *testing.T) {
	root := t.TempDir()
	full, ok := underRoot(root, pathClean("/foo/../../../etc/passwd"))
	if !ok {
		t.Fatal("expected path under root")
	}
	if full != filepath.Join(root, "etc", "passwd") && !strings.HasPrefix(full, filepath.Clean(root)+string(os.PathSeparator)) {
		t.Fatalf("escaped root: %s", full)
	}
}

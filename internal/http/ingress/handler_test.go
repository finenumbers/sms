package ingresshttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/ingress"
)

type fakeTokens struct {
	hash string
	err  error
}

func (f fakeTokens) IngressHash(context.Context) (string, error) {
	return f.hash, f.err
}

type fakeEvents struct {
	n    int
	last sqlcdb.InsertCallbackEventParams
	err  error
}

func (f *fakeEvents) InsertCallbackEvent(_ context.Context, arg sqlcdb.InsertCallbackEventParams) (sqlcdb.ProviderCallbackEvent, error) {
	f.n++
	f.last = arg
	if f.err != nil {
		return sqlcdb.ProviderCallbackEvent{}, f.err
	}
	return sqlcdb.ProviderCallbackEvent{ID: uuid.New()}, nil
}

func serveCapture(h *Handlers, method, path, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.HandleFunc("/internal/runexis/{kind}/{token}", h.Capture)
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCapturePersistThen200(t *testing.T) {
	token := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	ev := &fakeEvents{}
	h := &Handlers{
		Tokens: fakeTokens{hash: ingress.HashToken(token)},
		Events: ev,
	}
	rec := serveCapture(h, http.MethodPost, "/internal/runexis/dlr/"+token, `{"status":"delivered"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if ev.n != 1 {
		t.Fatalf("persist calls=%d", ev.n)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out["ok"] != true {
		t.Fatalf("body=%s", rec.Body.String())
	}
	if ev.last.Path != "/internal/runexis/dlr/*" {
		t.Fatalf("path stored=%q", ev.last.Path)
	}
	if strings.Contains(ev.last.Path, token) {
		t.Fatal("token leaked into stored path")
	}
}

func TestCapturePersistFailureIsNot200(t *testing.T) {
	token := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	ev := &fakeEvents{err: errors.New("db down")}
	h := &Handlers{
		Tokens: fakeTokens{hash: ingress.HashToken(token)},
		Events: ev,
	}
	rec := serveCapture(h, http.MethodPost, "/internal/runexis/mo/"+token, `{"text":"hi"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rec.Code)
	}
	if ev.n != 1 {
		t.Fatalf("persist calls=%d", ev.n)
	}
}

func TestCaptureDuplicateStill200(t *testing.T) {
	token := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	ev := &fakeEvents{err: pgx.ErrNoRows}
	h := &Handlers{
		Tokens: fakeTokens{hash: ingress.HashToken(token)},
		Events: ev,
	}
	rec := serveCapture(h, http.MethodPut, "/internal/runexis/dlr/"+token, `{"x":1}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestCaptureWrongTokenIs404AndDoesNotPersist(t *testing.T) {
	ev := &fakeEvents{}
	h := &Handlers{
		Tokens: fakeTokens{hash: ingress.HashToken("real-token")},
		Events: ev,
	}
	rec := serveCapture(h, http.MethodPost, "/internal/runexis/dlr/wrong-token", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
	if ev.n != 0 {
		t.Fatal("must not persist before token check")
	}
}

func TestCaptureGETQueryIsPayload(t *testing.T) {
	token := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	ev := &fakeEvents{}
	h := &Handlers{
		Tokens: fakeTokens{hash: ingress.HashToken(token)},
		Events: ev,
	}
	rec := serveCapture(h, http.MethodGet, "/internal/runexis/mo/"+token+"?from=79991112233&text=hi", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if ev.last.Query == nil || *ev.last.Query != "from=79991112233&text=hi" {
		t.Fatalf("query=%v", ev.last.Query)
	}
}

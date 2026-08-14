package publicapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"finenumbers/sms/internal/authctx"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	clienthttp "finenumbers/sms/internal/http/client"
	"finenumbers/sms/internal/httpx"
	"finenumbers/sms/internal/idempotency"
	"finenumbers/sms/internal/lookup"
	"finenumbers/sms/internal/messaging"
)

const maxSendBody = 1 << 20
const maxIdempotencyKey = 255

type Handlers struct {
	Log      *slog.Logger
	Store    *db.Store
	Messages *messaging.Service
	Lookup   *lookup.Service
}

type sendRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

func (h *Handlers) SendMessage(w http.ResponseWriter, r *http.Request) {
	p, ok := authctx.Principal(r.Context())
	if !ok || p.ClientID == nil || p.APIKeyID == nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	if h.Messages == nil || h.Store == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "unavailable", "messaging unavailable")
		return
	}

	raw, err := io.ReadAll(io.LimitReader(r.Body, maxSendBody+1))
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}
	if len(raw) > maxSendBody {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "request body too large")
		return
	}
	var req sendRequest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "invalid request body")
		return
	}

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	in := messaging.EnqueueInput{
		ClientID: *p.ClientID,
		From:     req.From,
		To:       req.To,
		Text:     req.Text,
	}
	if key == "" {
		msg, err := h.Messages.Enqueue(r.Context(), in)
		if err != nil {
			clienthttp.WriteMessageError(w, h.Log, err)
			return
		}
		httpx.WriteJSON(w, http.StatusAccepted, clienthttp.MessageJSON(msg))
		return
	}
	if len(key) > maxIdempotencyKey {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "Idempotency-Key too long")
		return
	}
	in.IdempotencyKey = &key

	tx, err := h.Store.Pool.Begin(r.Context())
	if err != nil {
		h.Log.Error("idempotency begin", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Store.Queries.WithTx(tx)

	rec, err := idempotency.Reserve(r.Context(), q, sqlcdb.ActorTypeApiKey, *p.APIKeyID, key, idempotency.HashRequest(r.Method, r.URL.Path, raw), idempotency.DefaultTTL)
	if err != nil {
		switch {
		case errors.Is(err, idempotency.ErrConflict):
			httpx.WriteError(w, http.StatusConflict, "idempotency_conflict", "Idempotency-Key reused with a different request")
		case errors.Is(err, idempotency.ErrInFlight):
			httpx.WriteError(w, http.StatusConflict, "idempotency_in_flight", "request with this Idempotency-Key is in progress")
		default:
			h.Log.Error("idempotency reserve", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		}
		return
	}
	if rec.Replay {
		if err := tx.Commit(r.Context()); err != nil {
			h.Log.Error("idempotency replay commit", "err", err)
			httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rec.Status)
		_, _ = w.Write(rec.Body)
		if len(rec.Body) == 0 || rec.Body[len(rec.Body)-1] != '\n' {
			_, _ = w.Write([]byte("\n"))
		}
		return
	}

	msg, err := h.Messages.EnqueueWith(r.Context(), q, in)
	if err != nil {
		clienthttp.WriteMessageError(w, h.Log, err)
		return
	}
	payload := clienthttp.MessageJSON(msg)
	body, err := json.Marshal(payload)
	if err != nil {
		h.Log.Error("idempotency marshal", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err := idempotency.Complete(r.Context(), q, rec.ID, http.StatusAccepted, body); err != nil {
		h.Log.Error("idempotency complete", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		h.Log.Error("idempotency commit", "err", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal error")
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, payload)
}

package ingress

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"finenumbers/sms/internal/billing"
	"finenumbers/sms/internal/db"
	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/metrics"
	"finenumbers/sms/internal/msisdn"
	"finenumbers/sms/internal/ops"
	"finenumbers/sms/internal/runexis"
)

const claimBatch = 20

type Worker struct {
	store   *db.Store
	billing *billing.Service
	log     *slog.Logger
	ops     *ops.Logger
}

func NewWorker(store *db.Store, bill *billing.Service, log *slog.Logger, opsLog *ops.Logger) *Worker {
	if log == nil {
		log = slog.Default()
	}
	return &Worker{store: store, billing: bill, log: log, ops: opsLog}
}

func (w *Worker) Tick(ctx context.Context) (int, error) {
	if w == nil || w.store == nil {
		return 0, nil
	}
	events, err := w.store.Queries.ListUnprocessedCallbackEvents(ctx, claimBatch)
	if err != nil {
		return 0, err
	}
	for i := range events {
		w.process(ctx, events[i])
	}
	return len(events), nil
}

func (w *Worker) process(ctx context.Context, ev sqlcdb.ProviderCallbackEvent) {
	ctx = ops.ContextWith(ctx, ops.Fields{
		RequestID:    "ingress:" + ev.ID.String(),
		ResourceType: "provider_callback_event",
		ResourceID:   &ev.ID,
	})
	ct := ""
	if ev.ContentType != nil {
		ct = *ev.ContentType
	}
	q := ""
	if ev.Query != nil {
		q = *ev.Query
	}
	rows := runexis.ParseCallbacks(q, ct, ev.RawBody)
	if len(rows) == 0 {
		w.finish(ctx, ev.ID, nil, map[string]any{"skipped": "unrecognized"})
		metrics.Callbacks.WithLabelValues(string(ev.Kind), "unrecognized").Inc()
		w.writeParsed(ctx, ev, "unrecognized", 0, nil, nil)
		return
	}
	var linked *uuid.UUID
	applied := 0
	for _, row := range rows {
		id, ok := w.apply(ctx, ev.Kind, row)
		if ok {
			applied++
			if linked == nil {
				linked = id
			}
		}
	}
	result := "applied"
	if applied == 0 {
		result = "no_match"
	}
	w.finish(ctx, ev.ID, linked, map[string]any{"result": result, "rows": rows, "applied": applied})
	metrics.Callbacks.WithLabelValues(string(ev.Kind), result).Inc()
	w.writeParsed(ctx, ev, result, applied, linked, rows)
}

func (w *Worker) apply(ctx context.Context, kind sqlcdb.CallbackKind, row runexis.Callback) (*uuid.UUID, bool) {
	switch kind {
	case sqlcdb.CallbackKindDlr:
		return w.applyDLR(ctx, row)
	case sqlcdb.CallbackKindMo:
		return w.applyMO(ctx, row)
	default:
		return nil, false
	}
}

func (w *Worker) applyDLR(ctx context.Context, row runexis.Callback) (*uuid.UUID, bool) {
	if row.SMSID == "" {
		return nil, false
	}
	msg, err := w.store.Queries.GetSmsMessageByProviderID(ctx, &row.SMSID)
	if err != nil {
		return nil, false
	}
	if row.Failed {
		st := row.Status
		if st == "" {
			st = "failed"
		}
		if _, err := w.store.Queries.UpdateSmsMessageFailed(ctx, sqlcdb.UpdateSmsMessageFailedParams{
			ProviderStatus: &st,
			ID:             msg.ID,
		}); err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
			w.log.Error("dlr fail", "id", msg.ID, "err", err)
			return nil, false
		}
		w.capture(ctx, msg.ID)
		return &msg.ID, true
	}
	var pdu *int32
	if row.PDU > 0 {
		v := int32(row.PDU)
		pdu = &v
	}
	var status *string
	if row.Status != "" {
		status = &row.Status
	}
	if _, err := w.store.Queries.UpdateSmsMessageFromStatistic(ctx, sqlcdb.UpdateSmsMessageFromStatisticParams{
		ProviderSmsID:  &row.SMSID,
		ProviderStatus: status,
		PduCount:       pdu,
		Delivered:      row.Delivered,
		Sent:           row.Sent,
		ID:             msg.ID,
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) && w.log != nil {
		w.log.Error("dlr apply", "id", msg.ID, "err", err)
		return nil, false
	}
	metrics.ObservePDUMismatch(msg.BilledSegments, pdu)
	w.capture(ctx, msg.ID)
	return &msg.ID, true
}

func (w *Worker) applyMO(ctx context.Context, row runexis.Callback) (*uuid.UUID, bool) {
	to := msisdn.Canonical(row.To)
	from := msisdn.Canonical(row.From)
	if to == "" || from == "" {
		return nil, false
	}
	text := row.Text
	var clientID *uuid.UUID
	if asg, err := w.store.Queries.GetOpenAssignmentByMSISDN(ctx, to); err == nil {
		clientID = &asg.ClientID
	}
	var pdu *int32
	if row.PDU > 0 {
		v := int32(row.PDU)
		pdu = &v
	}
	var sid *string
	if row.SMSID != "" {
		sid = &row.SMSID
	}
	msg, err := w.store.Queries.InsertInboundMessage(ctx, sqlcdb.InsertInboundMessageParams{
		ClientID:      clientID,
		FromMsisdn:    from,
		ToMsisdn:      to,
		Text:          text,
		ProviderSmsID: sid,
		PduCount:      pdu,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if sid != nil {
				if existing, err := w.store.Queries.GetSmsMessageByProviderID(ctx, sid); err == nil {
					return &existing.ID, true
				}
			}
			return nil, true
		}
		if w.log != nil {
			w.log.Error("mo apply", "err", err)
		}
		return nil, false
	}
	return &msg.ID, true
}

func (w *Worker) finish(ctx context.Context, id uuid.UUID, msgID *uuid.UUID, parsed any) {
	raw, err := json.Marshal(parsed)
	if err != nil {
		raw = []byte(`{"skipped":"marshal"}`)
	}
	if _, err := w.store.Queries.MarkCallbackProcessed(ctx, sqlcdb.MarkCallbackProcessedParams{
		Parsed:       raw,
		SmsMessageID: msgID,
		ID:           id,
	}); err != nil && w.log != nil {
		w.log.Error("mark callback processed", "id", id, "err", err)
	}
}

func (w *Worker) writeParsed(ctx context.Context, ev sqlcdb.ProviderCallbackEvent, result string, applied int, linked *uuid.UUID, rows []runexis.Callback) {
	if w.ops == nil {
		return
	}
	level := ops.LevelInfo
	if result != "applied" {
		level = ops.LevelWarn
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.SMSID != "" {
			ids = append(ids, row.SMSID)
		}
	}
	w.ops.Write(ctx, ops.Event{
		Category:     ops.CategoryIngress,
		Level:        level,
		Action:       "ingress.parsed",
		ResourceType: "provider_callback_event",
		ResourceID:   &ev.ID,
		Summary:      result,
		Detail: map[string]any{
			"callback_id":    ev.ID,
			"kind":           ev.Kind,
			"result":         result,
			"applied":        applied,
			"sms_id":         ids,
			"sms_message_id": linked,
		},
	})
}

func (w *Worker) capture(ctx context.Context, id uuid.UUID) {
	if w == nil || w.billing == nil {
		return
	}
	if err := w.billing.CaptureForMessage(ctx, id); err != nil && w.log != nil {
		w.log.Error("billing capture", "id", id, "err", err)
	}
}

package ops

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/netip"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
)

const (
	CategoryHTTP    = "http"
	CategoryDIDAPI  = "didapi"
	CategoryQueue   = "queue"
	CategoryIngress = "ingress"
	CategoryAudit   = "audit"

	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	writeTimeout = 150 * time.Millisecond
	detailLimit  = 64 << 10
)

type Logger struct {
	q   *sqlcdb.Queries
	log *slog.Logger
}

func New(q *sqlcdb.Queries, log *slog.Logger) *Logger {
	if log == nil {
		log = slog.Default()
	}
	return &Logger{q: q, log: log}
}

type Event struct {
	Category     string
	Level        string
	RequestID    string
	ActorType    string
	ActorID      *uuid.UUID
	ClientID     *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	HTTPMethod   string
	HTTPPath     string
	HTTPStatus   int
	LatencyMS    int32
	Summary      string
	Detail       any
	Error        string
	IP           *netip.Addr
}

func (l *Logger) Write(ctx context.Context, ev Event) {
	if l == nil || l.q == nil {
		return
	}
	mergeFromContext(&ev, ctx)
	if ev.Category == "" || ev.Action == "" {
		return
	}
	if ev.Level == "" {
		ev.Level = LevelInfo
	}
	detail := []byte("{}")
	if ev.Detail != nil {
		switch t := ev.Detail.(type) {
		case []byte:
			if len(t) > 0 {
				detail = t
			}
		case json.RawMessage:
			if len(t) > 0 {
				detail = t
			}
		default:
			if b, err := json.Marshal(ev.Detail); err == nil {
				detail = b
			}
		}
	}
	if len(detail) > detailLimit {
		detail = truncateJSON(detail, detailLimit)
	}

	ctx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	err := l.q.InsertOpsEvent(ctx, sqlcdb.InsertOpsEventParams{
		Category:     ev.Category,
		Level:        ev.Level,
		RequestID:    strPtr(ev.RequestID),
		ActorType:    nullActor(ev.ActorType),
		ActorID:      ev.ActorID,
		ClientID:     ev.ClientID,
		Action:       ev.Action,
		ResourceType: strPtr(ev.ResourceType),
		ResourceID:   ev.ResourceID,
		HttpMethod:   strPtr(ev.HTTPMethod),
		HttpPath:     strPtr(ev.HTTPPath),
		HttpStatus:   intPtr(ev.HTTPStatus),
		LatencyMs:    latencyPtr(ev),
		Summary:      strPtr(ev.Summary),
		Detail:       detail,
		Error:        strPtr(ev.Error),
		Ip:           ev.IP,
	})
	if err != nil && l.log != nil {
		l.log.Error("ops write failed", "action", ev.Action, "category", ev.Category, "err", err)
	}
}

func mergeFromContext(ev *Event, ctx context.Context) {
	f := From(ctx)
	if f == nil {
		return
	}
	if ev.RequestID == "" {
		ev.RequestID = f.RequestID
	}
	if ev.ActorType == "" {
		ev.ActorType = f.ActorType
	}
	if ev.ActorID == nil {
		ev.ActorID = f.ActorID
	}
	if ev.ClientID == nil {
		ev.ClientID = f.ClientID
	}
	if ev.ResourceType == "" {
		ev.ResourceType = f.ResourceType
	}
	if ev.ResourceID == nil {
		ev.ResourceID = f.ResourceID
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtr(n int) *int32 {
	if n == 0 {
		return nil
	}
	v := int32(n)
	return &v
}

func latencyPtr(ev Event) *int32 {
	if ev.Category != CategoryHTTP && ev.Category != CategoryDIDAPI {
		if ev.LatencyMS == 0 {
			return nil
		}
	}
	v := ev.LatencyMS
	return &v
}

func nullActor(s string) sqlcdb.NullActorType {
	if s == "" {
		return sqlcdb.NullActorType{}
	}
	return sqlcdb.NullActorType{ActorType: sqlcdb.ActorType(s), Valid: true}
}

func truncateJSON(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	previewMax := n - 96
	if previewMax < 32 {
		previewMax = 32
	}
	preview := b
	if len(preview) > previewMax {
		preview = preview[:previewMax]
		for len(preview) > 0 && !utf8.Valid(preview) {
			preview = preview[:len(preview)-1]
		}
	}
	out, err := json.Marshal(map[string]any{
		"_truncated": true,
		"bytes":      len(b),
		"preview":    string(preview),
	})
	if err != nil || !json.Valid(out) {
		return []byte(`{"_truncated":true}`)
	}
	return out
}

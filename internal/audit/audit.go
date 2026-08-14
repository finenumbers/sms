package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"

	sqlcdb "finenumbers/sms/internal/db/sqlc"
	"finenumbers/sms/internal/ops"
)

type Logger struct {
	q   *sqlcdb.Queries
	log *slog.Logger
	ops *ops.Logger
}

func New(q *sqlcdb.Queries, log *slog.Logger, opsLog *ops.Logger) *Logger {
	return &Logger{q: q, log: log, ops: opsLog}
}

type Event struct {
	ActorType    sqlcdb.ActorType
	ActorID      *uuid.UUID
	ClientID     *uuid.UUID
	Action       string
	ResourceType string
	ResourceID   *uuid.UUID
	IP           *netip.Addr
	UserAgent    *string
	Metadata     map[string]any
}

func (l *Logger) Write(ctx context.Context, ev Event) {
	if l == nil || l.q == nil {
		return
	}
	meta := []byte("{}")
	if ev.Metadata != nil {
		if b, err := json.Marshal(ev.Metadata); err == nil {
			meta = b
		}
	}
	if ev.ResourceType == "" {
		ev.ResourceType = "none"
	}
	err := l.q.InsertAuditLog(ctx, sqlcdb.InsertAuditLogParams{
		ActorType:    ev.ActorType,
		ActorID:      ev.ActorID,
		ClientID:     ev.ClientID,
		Action:       ev.Action,
		ResourceType: ev.ResourceType,
		ResourceID:   ev.ResourceID,
		Ip:           ev.IP,
		UserAgent:    ev.UserAgent,
		Metadata:     meta,
	})
	if err != nil {
		if l.log != nil {
			l.log.Error("audit write failed", "action", ev.Action, "err", err)
		}
		return
	}
	if l.ops != nil {
		l.ops.Write(ctx, ops.Event{
			Category:     ops.CategoryAudit,
			Level:        ops.LevelInfo,
			ActorType:    string(ev.ActorType),
			ActorID:      ev.ActorID,
			ClientID:     ev.ClientID,
			Action:       ev.Action,
			ResourceType: ev.ResourceType,
			ResourceID:   ev.ResourceID,
			Summary:      ev.Action,
			Detail:       ev.Metadata,
			IP:           ev.IP,
		})
	}
}

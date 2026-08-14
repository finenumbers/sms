package ops

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey struct{}

type Fields struct {
	RequestID    string
	ActorType    string
	ActorID      *uuid.UUID
	ClientID     *uuid.UUID
	ResourceType string
	ResourceID   *uuid.UUID
}

func Attach(ctx context.Context) context.Context {
	if From(ctx) != nil {
		return ctx
	}
	return context.WithValue(ctx, ctxKey{}, &Fields{})
}

func From(ctx context.Context) *Fields {
	if ctx == nil {
		return nil
	}
	f, _ := ctx.Value(ctxKey{}).(*Fields)
	return f
}

func ContextWith(ctx context.Context, patch Fields) context.Context {
	f := From(ctx)
	if f == nil {
		ctx = Attach(ctx)
		f = From(ctx)
	}
	if patch.RequestID != "" {
		f.RequestID = patch.RequestID
	}
	if patch.ActorType != "" {
		f.ActorType = patch.ActorType
	}
	if patch.ActorID != nil {
		f.ActorID = patch.ActorID
	}
	if patch.ClientID != nil {
		f.ClientID = patch.ClientID
	}
	if patch.ResourceType != "" {
		f.ResourceType = patch.ResourceType
	}
	if patch.ResourceID != nil {
		f.ResourceID = patch.ResourceID
	}
	return ctx
}

func NoteActor(ctx context.Context, actorType string, actorID, clientID *uuid.UUID) {
	f := From(ctx)
	if f == nil {
		return
	}
	f.ActorType = actorType
	f.ActorID = actorID
	if clientID != nil {
		f.ClientID = clientID
	}
}

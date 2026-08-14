package authctx

import (
	"context"

	"finenumbers/sms/internal/identity"
	"finenumbers/sms/internal/ops"
)

type key struct{}

func WithPrincipal(ctx context.Context, p identity.Principal) context.Context {
	ctx = context.WithValue(ctx, key{}, p)
	at, aid := p.AuditActor()
	ops.NoteActor(ctx, string(at), aid, p.ClientID)
	return ctx
}

func Principal(ctx context.Context) (identity.Principal, bool) {
	p, ok := ctx.Value(key{}).(identity.Principal)
	return p, ok
}

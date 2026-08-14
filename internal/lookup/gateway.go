package lookup

import (
	"context"

	"finenumbers/sms/internal/smsc"
)

type Provider interface {
	Configured() bool
	SubmitHLR(ctx context.Context, in smsc.SubmitInput) (smsc.SubmitResult, error)
	SubmitPing(ctx context.Context, in smsc.SubmitInput) (smsc.SubmitResult, error)
	FetchStatus(ctx context.Context, in smsc.FetchStatusInput) (smsc.FetchStatusResult, error)
}

type Gateway struct {
	p *smsc.Provider
}

func NewGateway(p *smsc.Provider) *Gateway {
	if p == nil {
		return nil
	}
	return &Gateway{p: p}
}

func (g *Gateway) Configured() bool {
	return g != nil && g.p != nil && g.p.Configured()
}

func (g *Gateway) SubmitHLR(ctx context.Context, in smsc.SubmitInput) (smsc.SubmitResult, error) {
	return g.p.SubmitHLR(ctx, in)
}

func (g *Gateway) SubmitPing(ctx context.Context, in smsc.SubmitInput) (smsc.SubmitResult, error) {
	return g.p.SubmitPing(ctx, in)
}

func (g *Gateway) FetchStatus(ctx context.Context, in smsc.FetchStatusInput) (smsc.FetchStatusResult, error) {
	return g.p.FetchStatus(ctx, in)
}

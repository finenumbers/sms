package runexis

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
)

func (c *Client) SetDLRURL(ctx context.Context, rawURL string) error {
	return c.setCallbackURL(ctx, "/api/v1/sms/dlr-url", rawURL)
}

func (c *Client) SetHookURL(ctx context.Context, rawURL string) error {
	return c.setCallbackURL(ctx, "/api/v1/sms/hook-url", rawURL)
}

type wireCallbackURL struct {
	URL string `json:"url"`
}

func (c *Client) setCallbackURL(ctx context.Context, path, rawURL string) error {
	var env wireEnvelope
	if _, err := c.doJSON(ctx, http.MethodPatch, path, wireCallbackURL{URL: rawURL}, true, &env); err != nil {
		return err
	}
	if !env.Success {
		return envelopeError(0, env)
	}
	return nil
}

func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var nerr net.Error
	if errors.As(err, &nerr) && nerr.Timeout() {
		return true
	}
	return false
}

func HTTPStatus(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

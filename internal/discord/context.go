package discord

import "context"

// ctxKey is the private type for context values in this package.
type ctxKey int

const clientKey ctxKey = iota

// WithClient stashes the REST client that should serve the current unit of work
// (one gateway event or interaction) in the context. The worker injects the
// custom bot's client, resolved from the event's app id, so downstream code can
// act as the right bot without a per-call database lookup.
func WithClient(ctx context.Context, c *Client) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, clientKey, c)
}

// ClientFromContext returns the client injected by WithClient, or nil.
func ClientFromContext(ctx context.Context) *Client {
	if ctx == nil {
		return nil
	}
	if c, ok := ctx.Value(clientKey).(*Client); ok {
		return c
	}
	return nil
}

package tools

import "context"

// AskFunc asks the user a question and blocks until an answer (or ctx is done).
// It is supplied by the interactive chat layer; autonomous runs leave it unset.
type AskFunc func(ctx context.Context, question string, options []string) (string, error)

// askKey keys the AskFunc on a request context.
type askKey struct{}

// WithAsker attaches an interactive asker to ctx so the ask_user tool can prompt
// the user mid-turn. Kept in the tools package (not agent) so built-in tools can
// reach it without importing the agent package (which would cycle).
func WithAsker(ctx context.Context, fn AskFunc) context.Context {
	return context.WithValue(ctx, askKey{}, fn)
}

// askerFrom returns the asker attached to ctx, or nil when none is present
// (e.g. heartbeat/scheduler runs with no open client connection).
func askerFrom(ctx context.Context) AskFunc {
	fn, _ := ctx.Value(askKey{}).(AskFunc)
	return fn
}

package tools

import "context"

// curSessionKey carries the current turn's session id so session-scoped tools
// (e.g. read_session_debug) can default to "the session I am running in" without
// the caller passing it explicitly. Kept in the tools package so built-in tools
// reach it without importing agent/api (which would cycle).
type curSessionKey struct{}

// WithCurrentSession attaches the current session id to ctx for the duration of a
// turn. The runtime sets it once at the top of the turn.
func WithCurrentSession(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, curSessionKey{}, sessionID)
}

// CurrentSessionID returns the session id attached to ctx, or "" when none.
func CurrentSessionID(ctx context.Context) string {
	s, _ := ctx.Value(curSessionKey{}).(string)
	return s
}

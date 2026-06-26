package tools

import "context"

// SessionSink groups the session-scoped mutations an agent can make to its OWN
// session: rename it, set its working directory, archive it. It is a superset of
// GoalSink (the goal tools depend on the narrower interface) — one concrete sink
// bound to the session implements both. Turns without a sink make these tools
// graceful no-ops.
//
// Kept in the tools package (not agent/api) so built-in tools can reach it
// without importing those packages (which would cycle).
type SessionSink interface {
	GoalSink
	SetTitle(ctx context.Context, title string) error
	SetWorkingDir(ctx context.Context, dir string) error
	Archive(ctx context.Context) error
}

type sessionKey struct{}

// WithSession attaches a session sink to ctx so the set_session_title /
// set_working_dir / archive_session tools can mutate the current session.
func WithSession(ctx context.Context, sink SessionSink) context.Context {
	return context.WithValue(ctx, sessionKey{}, sink)
}

// sessionFrom returns the sink attached to ctx, or nil when none is present.
func sessionFrom(ctx context.Context) SessionSink {
	s, _ := ctx.Value(sessionKey{}).(SessionSink)
	return s
}

// HasSessionSink reports whether a session sink is attached to ctx.
func HasSessionSink(ctx context.Context) bool { return sessionFrom(ctx) != nil }

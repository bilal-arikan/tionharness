package tools

import "context"

// NotifySpec is a single desktop notification an agent raises mid-turn via the
// notify tool. Level is one of "info", "success", "error" (defaulting to "info"
// when empty), matching events.Event.Level so the UI styles the toast.
type NotifySpec struct {
	Title string
	Body  string
	Level string
}

// NotifySink delivers an agent-raised notification to the user's UI (a
// workspace-scoped event that becomes an OS toast, per device prefs). It is
// supplied by the chat layer carrying the origin session + agent; autonomous
// runs without a sink make the notify tool a graceful no-op.
//
// Kept in the tools package (not agent/api) so built-in tools can reach it
// without importing those packages (which would cycle).
type NotifySink interface {
	Notify(ctx context.Context, spec NotifySpec) error
}

type notifyKey struct{}

// WithNotify attaches a notify sink to ctx so the notify tool can raise a
// desktop notification mid-turn.
func WithNotify(ctx context.Context, sink NotifySink) context.Context {
	return context.WithValue(ctx, notifyKey{}, sink)
}

// notifyFrom returns the sink attached to ctx, or nil when none is present
// (e.g. scheduler runs with no open client connection).
func notifyFrom(ctx context.Context) NotifySink {
	s, _ := ctx.Value(notifyKey{}).(NotifySink)
	return s
}

// HasNotifySink reports whether a notify sink is attached to ctx, so a caller
// can install a fallback only when one is missing.
func HasNotifySink(ctx context.Context) bool { return notifyFrom(ctx) != nil }

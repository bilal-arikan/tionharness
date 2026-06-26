package tools

import "context"

// NavigateSpec is one agent-driven UI navigation request raised via the
// focus_view tool. View is a NavRail view name (chat/agents/board/flows/...);
// SessionID/AgentID are optional entity hints — when empty the sink fills the
// current turn's session/agent so "focus_view chat" focuses this very session.
type NavigateSpec struct {
	View      string
	SessionID string
	AgentID   string
}

// NavigateSink drives the user's UI to a view/entity (a "navigate" event that
// open windows apply immediately, switching workspace/view and selecting the
// entity). Supplied by the chat layer carrying the origin session + agent;
// autonomous runs without a sink make focus_view a graceful no-op.
//
// Kept in the tools package (not agent/api) so built-in tools can reach it
// without importing those packages (which would cycle).
type NavigateSink interface {
	Navigate(ctx context.Context, spec NavigateSpec) error
}

type navigateKey struct{}

// WithNavigate attaches a navigate sink to ctx so the focus_view tool can drive
// the UI mid-turn.
func WithNavigate(ctx context.Context, sink NavigateSink) context.Context {
	return context.WithValue(ctx, navigateKey{}, sink)
}

// navigateFrom returns the sink attached to ctx, or nil when none is present
// (e.g. scheduler runs with no open client connection).
func navigateFrom(ctx context.Context) NavigateSink {
	s, _ := ctx.Value(navigateKey{}).(NavigateSink)
	return s
}

// HasNavigateSink reports whether a navigate sink is attached to ctx.
func HasNavigateSink(ctx context.Context) bool { return navigateFrom(ctx) != nil }

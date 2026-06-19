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
// (e.g. scheduler runs with no open client connection).
func askerFrom(ctx context.Context) AskFunc {
	fn, _ := ctx.Value(askKey{}).(AskFunc)
	return fn
}

// AskerFrom is the exported view of askerFrom: it lets the permission gate (in
// the agent package) reuse the interactive ask channel to prompt for tool
// approval. Returns nil on autonomous runs with no open client connection.
func AskerFrom(ctx context.Context) AskFunc { return askerFrom(ctx) }

// autonomousKey marks a context as belonging to a non-interactive (autonomous)
// run — schedule, flow, delegate, etc.
type autonomousKey struct{}

// WithAutonomous stamps ctx as autonomous (no interactive user present). The
// agent runtime calls this once at every non-chat entry point via WithCallKind
// so tools can detect the run kind before touching the input payload.
func WithAutonomous(ctx context.Context) context.Context {
	return context.WithValue(ctx, autonomousKey{}, true)
}

// IsAutonomous reports whether ctx was stamped as autonomous. Interactive-only
// tools (ask_user, request_confirmation) check this before unmarshalling input
// so a malformed or array-vs-string payload in a non-interactive run never
// surfaces a JSON error to the model.
func IsAutonomous(ctx context.Context) bool {
	v, _ := ctx.Value(autonomousKey{}).(bool)
	return v
}

// asyncChatKey marks a context as an autonomous run that nonetheless delivers
// into a real, human-visible chat session — e.g. a schedule_wake turn re-invoked
// into its originating conversation. The user is present asynchronously: they
// cannot answer a blocking ask_user mid-turn, but they CAN read the turn's reply
// and respond in the chat afterwards.
type asyncChatKey struct{}

// WithAsyncChat stamps ctx as an asynchronous chat run (see asyncChatKey). The
// wake delivery path sets this so interactive-only tools can give the model
// chat-appropriate guidance ("ask in your reply, end the turn") instead of the
// fully-headless "proceed without asking".
func WithAsyncChat(ctx context.Context) context.Context {
	return context.WithValue(ctx, asyncChatKey{}, true)
}

// IsAsyncChat reports whether ctx is an asynchronous chat run (a wake-delivered
// turn into a live session) rather than a fully headless one.
func IsAsyncChat(ctx context.Context) bool {
	v, _ := ctx.Value(asyncChatKey{}).(bool)
	return v
}

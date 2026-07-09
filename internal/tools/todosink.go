package tools

import "context"

// TodoSinkItem mirrors a single todo_write checklist entry handed to a sink.
// Kept local to the tools package (not importing internal/progress) so built-in
// tools stay dependency-light and free of import cycles. Category and Steps are
// the optional richer feature_list fields, carried through to the progress file.
type TodoSinkItem struct {
	Content  string   `json:"content"`
	Status   string   `json:"status"`
	Category string   `json:"category,omitempty"`
	Steps    []string `json:"steps,omitempty"`
}

// TodoSink persists the agent's working checklist to durable storage as it is
// updated, so the list survives across sessions (Claude Code's claude-progress
// convention). It is supplied by the chat/autonomous layer carrying the origin
// session, agent and working directory; when absent, todo_write simply skips
// persistence (the live UI checklist still works).
//
// Lives in the tools package so built-in tools can reach it without importing
// agent/api (which would cycle).
type TodoSink interface {
	SaveTodos(ctx context.Context, todos []TodoSinkItem) error
	// LoadTodos returns the last persisted checklist (ok=false when none exists
	// yet). It backs todo_write's compact `set` update form, which needs the
	// previous list to merge status changes into.
	LoadTodos(ctx context.Context) (todos []TodoSinkItem, ok bool, err error)
}

type todoSinkKey struct{}

// WithTodoSink attaches a todo sink to ctx so todo_write can persist its list.
func WithTodoSink(ctx context.Context, sink TodoSink) context.Context {
	return context.WithValue(ctx, todoSinkKey{}, sink)
}

// todoSinkFrom returns the sink attached to ctx, or nil when none is present.
func todoSinkFrom(ctx context.Context) TodoSink {
	s, _ := ctx.Value(todoSinkKey{}).(TodoSink)
	return s
}

// HasTodoSink reports whether a todo sink is attached to ctx, so a caller can
// install a fallback only when one is missing.
func HasTodoSink(ctx context.Context) bool { return todoSinkFrom(ctx) != nil }

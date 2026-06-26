package tools

import "context"

// GoalState is a session's persistent "north star": the objective text and
// whether it is marked done (a completed goal stays stored but stops steering
// future turns). Mirrors db.Session.Goal / GoalDone.
type GoalState struct {
	Text string
	Done bool
}

// GoalSink reads and writes the current session's persistent goal — the SAME
// field the user sets in the UI (db.Session.Goal), so the agent and the user
// share one north star rather than maintaining parallel objectives. Supplied by
// the chat/autonomous layer carrying the origin session; turns without a sink
// make the goal tools graceful no-ops.
//
// Kept in the tools package (not agent/api) so built-in tools can reach it
// without importing those packages (which would cycle).
type GoalSink interface {
	Goal(ctx context.Context) (GoalState, error)
	SetGoal(ctx context.Context, text string, done bool) error
}

type goalKey struct{}

// WithGoal attaches a goal sink to ctx so set_session_goal / complete_goal can
// read and write the session's persistent objective.
func WithGoal(ctx context.Context, sink GoalSink) context.Context {
	return context.WithValue(ctx, goalKey{}, sink)
}

// goalFrom returns the sink attached to ctx, or nil when none is present.
func goalFrom(ctx context.Context) GoalSink {
	s, _ := ctx.Value(goalKey{}).(GoalSink)
	return s
}

// HasGoalSink reports whether a goal sink is attached to ctx.
func HasGoalSink(ctx context.Context) bool { return goalFrom(ctx) != nil }

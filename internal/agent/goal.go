package agent

import (
	"context"
	"strings"
)

// GoalContextBlock renders a session's persistent objective as a system-prompt
// section. Inspired by Claude Code's /goal: a single durable "north star" the
// agent should keep steering toward across turns. Kept in the dynamic (uncached)
// suffix and placed first so it leads the volatile context. Returns "" when no
// goal is set OR when the goal is marked done (a completed objective stops
// steering future turns — once achieved, drop it).
//
// Lives in the agent package so BOTH the chat path (api/chat_turn.go) and the
// autonomous path (executor.go) render the goal identically — single source.
func GoalContextBlock(goal string, done bool) string {
	goal = strings.TrimSpace(goal)
	if goal == "" || done {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Session goal (north star)\n")
	b.WriteString("Keep every reply aligned with this persistent objective and make steady progress toward it; flag when it is achieved or blocked. It persists across turns.\n\n")
	b.WriteString(goal)
	return strings.TrimSpace(b.String())
}

// autonomousGoalBlock resolves the session bound to ctx (scheduler/spawn/peer
// turns stamp it via WithSessionID) and renders its goal block, so headless runs
// steer toward the session's north star just like chat turns do. Returns "" when
// there is no session in ctx or no active goal — a safe no-op for fresh spawns.
func (r *Runtime) autonomousGoalBlock(ctx context.Context) string {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return ""
	}
	sess, err := r.db.GetSession(ctx, sid)
	if err != nil {
		return ""
	}
	return GoalContextBlock(sess.Goal, sess.GoalDone)
}

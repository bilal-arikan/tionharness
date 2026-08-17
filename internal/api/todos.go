package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/progress"
	"github.com/bilal-arikan/tionswarm/internal/view"
)

// todoTailWindow is how many trailing messages the checklist lookup reads before
// widening to the full transcript. A live checklist is rewritten by todo_write
// throughout a turn, so the newest one is near the end in practice.
const todoTailWindow = 64

// todoContextBlock builds a system-prompt section showing the session's active
// todo checklist (the latest todo_write), so the agent keeps tracking it even
// after the original tool message has scrolled out of the live context window or
// been folded into the rolling summary by compaction. Returns "" when there is
// no active list (none yet, or the latest is fully completed). Kept in the
// dynamic (uncached) suffix since it changes whenever the list is updated.
//
// When the session has no checklist of its own yet (a fresh reload) and resume
// is enabled, it falls back to the durable per-session progress file at dir
// (resolved by agent.Runtime.ProgressDir) — so the agent picks up THIS session's
// own checklist after a context reset. Progress is session-specific, not shared
// across sessions on a working directory.
func todoContextBlock(ctx context.Context, database *db.DB, sessionID, dir string, resume bool) string {
	if sessionID == "" {
		return ""
	}
	// LatestTodos scans newest-first and stops at the first checklist, so a tail
	// answers it in almost every case. When the tail is truncated AND holds no
	// checklist the question is still open — fall back to the full transcript
	// rather than reporting "no checklist" for a session that has one.
	if msgs, from, err := database.ListMessagesTail(ctx, sessionID, todoTailWindow); err == nil {
		if own := view.LatestTodos(msgs); !own.Empty() {
			return renderTodoBlock(own)
		}
		if from > 0 {
			if all, aerr := database.ListMessages(ctx, sessionID); aerr == nil {
				if own := view.LatestTodos(all); !own.Empty() {
					return renderTodoBlock(own)
				}
			}
		}
	}
	if !resume {
		return ""
	}
	rec, ok, err := progress.Load(dir)
	if err != nil || !ok {
		return ""
	}
	return renderResumedBlock(rec)
}

// renderResumedBlock formats a previous session's persisted checklist as a
// resume hint for a fresh session. Returns "" for an empty or fully-completed
// list (nothing left to resume).
func renderResumedBlock(rec progress.Record) string {
	if len(rec.Todos) == 0 {
		return ""
	}
	items := make([]agent.TodoItem, len(rec.Todos))
	allDone := true
	for i, t := range rec.Todos {
		items[i] = agent.TodoItem{Content: t.Content, Status: t.Status}
		if t.Status != "completed" {
			allDone = false
		}
	}
	if allDone {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Resumed progress (from a previous session)\n")
	b.WriteString("This checklist was persisted by an earlier session working on this project. Continue from where it left off; keep it current by calling todo_write as you start and finish items.\n")
	for _, t := range items {
		mark := " "
		switch t.Status {
		case "completed":
			mark = "x"
		case "in_progress":
			mark = "~"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", mark, t.Content)
	}
	return strings.TrimSpace(b.String())
}

// renderTodoBlock formats a checklist as the system-prompt section. Returns ""
// for an empty list or one that is fully completed (nothing left to track —
// mirrors the UI, which hides a completed list).
//
// The checklist ITSELF is rendered by view.TodoRollup (shared with the handoff
// environment snapshot); only the heading and the "how to update it" instruction
// are specific to this surface.
func renderTodoBlock(r view.TodoRollup) string {
	if r.Empty() || r.AllDone() {
		return ""
	}
	return "## Active todo list (this session)\n" +
		"This is the checklist you are tracking with the todo_write tool. It persists here even if the original message has scrolled out of context. Keep it current: flip statuses with the compact form todo_write {\"set\":{\"<index>\":\"<status>\"}} using the 1-based indices below.\n" +
		r.RenderChecklist()
}

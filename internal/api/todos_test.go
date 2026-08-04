package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/progress"
	"github.com/bilal-arikan/tionswarm/internal/view"
)

func todoMsg(steps string) db.Message { return db.Message{Role: "assistant", Steps: steps} }

// The checklist scan and the [x]/[~]/[ ] rendering now live in internal/view
// (shared with the session projection and the handoff snapshot), so their own
// tests live there. What remains here is this surface's specific contract: the
// heading, the update instruction, and hiding a finished list.
func TestRenderTodoBlock(t *testing.T) {
	msgs := []db.Message{
		{Role: "user", Steps: ""},
		todoMsg(`[{"kind":"todo","todos":[{"content":"a","status":"completed"}]}]`),
		// Newer list supersedes the older one.
		todoMsg(`[{"kind":"todo","todos":[
			{"content":"read","status":"completed"},
			{"content":"work","status":"in_progress"},
			{"content":"write","status":"pending"}]}]`),
	}
	out := renderTodoBlock(view.LatestTodos(msgs))

	// Numbered lines: the model references these 1-based indices in the
	// compact todo_write {"set":{...}} update form.
	for _, want := range []string{"1. [x] read", "2. [~] work", "3. [ ] write", `{"set":`, "Active todo list"} {
		if !strings.Contains(out, want) {
			t.Fatalf("block missing %q\n%s", want, out)
		}
	}
	// The superseded older list must not leak in.
	if strings.Contains(out, "[x] a") {
		t.Fatalf("older checklist leaked into the block:\n%s", out)
	}

	// All-completed → empty (nothing left to track).
	done := view.LatestTodos([]db.Message{
		todoMsg(`[{"kind":"todo","todos":[{"content":"done","status":"completed"}]}]`)})
	if renderTodoBlock(done) != "" {
		t.Fatal("expected empty block for all-completed list")
	}
	// No checklist at all → empty.
	if renderTodoBlock(view.LatestTodos(nil)) != "" {
		t.Fatal("expected empty block when the session has no checklist")
	}
}

func TestRenderResumedBlock(t *testing.T) {
	rec := progress.Record{Todos: []progress.TodoItem{
		{Content: "ported", Status: "completed"},
		{Content: "wire ui", Status: "pending"},
	}}
	out := renderResumedBlock(rec)
	for _, want := range []string{"Resumed progress", "- [x] ported", "- [ ] wire ui", "previous session"} {
		if !strings.Contains(out, want) {
			t.Fatalf("resumed block missing %q\n%s", want, out)
		}
	}
	// All-completed → empty (nothing left to resume).
	if renderResumedBlock(progress.Record{Todos: []progress.TodoItem{{Content: "x", Status: "completed"}}}) != "" {
		t.Fatal("expected empty resumed block for all-completed list")
	}
	// Empty record → empty.
	if renderResumedBlock(progress.Record{}) != "" {
		t.Fatal("expected empty resumed block for empty record")
	}
}

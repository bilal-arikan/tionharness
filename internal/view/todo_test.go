package view

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func msgWithSteps(steps string) db.Message { return db.Message{Role: "assistant", Steps: steps} }

func TestLatestTodosPicksNewestList(t *testing.T) {
	msgs := []db.Message{
		{Role: "user"},
		msgWithSteps(`[{"kind":"todo","todos":[{"content":"eski","status":"pending"}]}]`),
		{Role: "user"},
		// todo_write replaces the list wholesale, so the older one is superseded
		// state — not extra state to merge.
		msgWithSteps(`[{"kind":"todo","todos":[
			{"content":"a","status":"completed"},
			{"content":"b","status":"in_progress"},
			{"content":"c","status":"pending"}]}]`),
	}
	r := LatestTodos(msgs)

	if len(r.Items) != 3 {
		t.Fatalf("got %d items, want 3 (newest list only)", len(r.Items))
	}
	if r.Done != 1 {
		t.Errorf("Done = %d, want 1", r.Done)
	}
	if r.Active != "b" {
		t.Errorf("Active = %q, want b", r.Active)
	}
	for _, it := range r.Items {
		if it.Content == "eski" {
			t.Errorf("superseded list leaked in: %+v", r.Items)
		}
	}
}

func TestLatestTodosEmptyStates(t *testing.T) {
	if r := LatestTodos(nil); !r.Empty() {
		t.Errorf("nil messages should yield an empty rollup, got %+v", r)
	}
	if r := LatestTodos([]db.Message{{Role: "user"}, msgWithSteps(`[{"kind":"text"}]`)}); !r.Empty() {
		t.Errorf("a session with no checklist should yield an empty rollup, got %+v", r)
	}
	// Empty is NOT the same as all-done: a session that never had a list must not
	// look like one that finished its work.
	if LatestTodos(nil).AllDone() {
		t.Error("an empty rollup must not report AllDone")
	}
}

func TestTodoRollupAllDone(t *testing.T) {
	done := LatestTodos([]db.Message{msgWithSteps(
		`[{"kind":"todo","todos":[{"content":"a","status":"completed"},{"content":"b","status":"completed"}]}]`)})
	if !done.AllDone() {
		t.Errorf("a fully completed list must report AllDone: %+v", done)
	}
	if done.Active != "" {
		t.Errorf("a finished list has nothing in flight, got %q", done.Active)
	}

	partial := LatestTodos([]db.Message{msgWithSteps(
		`[{"kind":"todo","todos":[{"content":"a","status":"completed"},{"content":"b","status":"pending"}]}]`)})
	if partial.AllDone() {
		t.Errorf("a partial list must not report AllDone: %+v", partial)
	}
}

func TestRenderChecklistCarriesActionableIndices(t *testing.T) {
	r := LatestTodos([]db.Message{msgWithSteps(`[{"kind":"todo","todos":[
		{"content":"read","status":"completed"},
		{"content":"work","status":"in_progress"},
		{"content":"write","status":"pending"}]}]`)})

	out := r.RenderChecklist()
	// The 1-based indices are the handle todo_write {"set":{"3":"completed"}}
	// refers to — a renderer that dropped them would leave the agent unable to
	// update the list it was just shown.
	for _, want := range []string{"1. [x] read", "2. [~] work", "3. [ ] write"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.HasSuffix(out, "\n") {
		t.Errorf("checklist should not end with a newline: %q", out)
	}
}

// TestRenderChecklistKeepsCompletedLists: unlike the system-prompt block (which
// hides a finished list), the renderer itself keeps it. The handoff needs it —
// "these are already done" is what stops a fresh agent redoing them.
func TestRenderChecklistKeepsCompletedLists(t *testing.T) {
	r := LatestTodos([]db.Message{msgWithSteps(
		`[{"kind":"todo","todos":[{"content":"bitti","status":"completed"}]}]`)})
	if out := r.RenderChecklist(); !strings.Contains(out, "1. [x] bitti") {
		t.Errorf("completed item dropped by the renderer: %q", out)
	}
	if out := (TodoRollup{}).RenderChecklist(); out != "" {
		t.Errorf("empty rollup should render nothing, got %q", out)
	}
}

func TestLatestTodosReadsLegacyToolInput(t *testing.T) {
	r := LatestTodos([]db.Message{msgWithSteps(
		`[{"kind":"tool","tool":"todo_write","input":{"todos":[{"content":"eski form","status":"pending"}]}}]`)})
	if len(r.Items) != 1 || r.Items[0].Content != "eski form" {
		t.Errorf("legacy todo_write input form not read: %+v", r.Items)
	}
}

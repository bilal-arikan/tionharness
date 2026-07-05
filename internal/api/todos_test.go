package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/progress"
)

func todoMsg(steps string) db.Message { return db.Message{Role: "assistant", Steps: steps} }

func TestLatestSessionTodos_PicksNewest(t *testing.T) {
	msgs := []db.Message{
		{Role: "user", Steps: ""},
		todoMsg(`[{"kind":"todo","todos":[{"content":"a","status":"completed"},{"content":"b","status":"pending"}]}]`),
		{Role: "user", Steps: ""},
		// Newer list supersedes the older one.
		todoMsg(`[{"kind":"todo","todos":[{"content":"x","status":"in_progress"}]}]`),
	}
	got := latestSessionTodos(msgs)
	if len(got) != 1 || got[0].Content != "x" || got[0].Status != "in_progress" {
		t.Fatalf("expected newest single in_progress item, got %+v", got)
	}
}

func TestStepTodos_LegacyToolInput(t *testing.T) {
	// A todo carried as a todo_write tool step's input (no typed Todos field).
	step := agent.TurnStep{Kind: "tool", Tool: "todo_write", Input: []byte(`{"todos":[{"content":"c","status":"pending"}]}`)}
	got := stepTodos(step)
	if len(got) != 1 || got[0].Content != "c" {
		t.Fatalf("expected legacy todo parse, got %+v", got)
	}
	// A non-todo step yields nothing.
	if out := stepTodos(agent.TurnStep{Kind: "tool", Tool: "bash"}); out != nil {
		t.Fatalf("expected nil for non-todo step, got %+v", out)
	}
}

func TestRenderTodoBlock(t *testing.T) {
	todos := []agent.TodoItem{
		{Content: "read", Status: "completed"},
		{Content: "work", Status: "in_progress"},
		{Content: "write", Status: "pending"},
	}
	out := renderTodoBlock(todos)
	for _, want := range []string{"- [x] read", "- [~] work", "- [ ] write", "Active todo list"} {
		if !strings.Contains(out, want) {
			t.Fatalf("block missing %q\n%s", want, out)
		}
	}
	// All-completed → empty (nothing left to track).
	if renderTodoBlock([]agent.TodoItem{{Content: "done", Status: "completed"}}) != "" {
		t.Fatal("expected empty block for all-completed list")
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

package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/progress"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// TestNewTodoSinkPersistsToCwd verifies the todo sink writes the checklist to the
// project's progress file when a working directory is given.
func TestNewTodoSinkPersistsToCwd(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	cwd := t.TempDir()

	sink := rt.NewTodoSink("SES1", "AGT1", cwd)
	err := sink.SaveTodos(context.Background(), []tools.TodoSinkItem{
		{Content: "build", Status: "completed"},
		{Content: "test", Status: "in_progress"},
	})
	if err != nil {
		t.Fatalf("save todos: %v", err)
	}

	rec, ok, err := progress.Load(cwd)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	if rec.SessionID != "SES1" || rec.AgentID != "AGT1" {
		t.Fatalf("ids not stamped: %+v", rec)
	}
	if len(rec.Todos) != 2 || rec.Todos[0].Status != "completed" {
		t.Fatalf("todos not persisted: %+v", rec.Todos)
	}
	if len(rec.Log) != 1 {
		t.Fatalf("expected one log entry, got %d", len(rec.Log))
	}
}

// TestNewTodoSinkFallsBackToStore verifies that with no cwd the sink writes to a
// per-agent file under the workspace store, so an agent still resumes its own
// progress.
func TestNewTodoSinkFallsBackToStore(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	sink := rt.NewTodoSink("SES2", "AGT9", "")
	if err := sink.SaveTodos(context.Background(), []tools.TodoSinkItem{{Content: "x", Status: "pending"}}); err != nil {
		t.Fatalf("save todos: %v", err)
	}

	fallback := filepath.Join(rt.db.Root(), "progress", "AGT9")
	rec, ok, err := progress.Load(fallback)
	if err != nil || !ok {
		t.Fatalf("load fallback: ok=%v err=%v", ok, err)
	}
	if len(rec.Todos) != 1 || rec.Todos[0].Content != "x" {
		t.Fatalf("fallback todos wrong: %+v", rec.Todos)
	}
}

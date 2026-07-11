package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/progress"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// TestNewTodoSinkIsPerSessionEvenWithSharedCwd verifies the checklist persists to
// a PER-SESSION file — NOT the working directory — so two sessions sharing the
// same project dir keep independent progress (the dir-shared file is gone).
func TestNewTodoSinkIsPerSessionEvenWithSharedCwd(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	cwd := t.TempDir()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID, WorkingDir: cwd})

	sink := rt.NewTodoSink(sess.ID, ag.ID)
	if err := sink.SaveTodos(ctx, []tools.TodoSinkItem{
		{Content: "build", Status: "completed"},
		{Content: "test", Status: "in_progress"},
	}); err != nil {
		t.Fatalf("save todos: %v", err)
	}

	// Must NOT leak into the working directory (no dir-shared progress file).
	if _, ok, _ := progress.Load(cwd); ok {
		t.Fatalf("progress leaked into the shared working dir %s (want per-session only)", cwd)
	}
	// It lives in the per-session store dir, stamped with this session.
	rec, ok, err := progress.Load(rt.ProgressDir(sess.ID))
	if err != nil || !ok {
		t.Fatalf("load per-session: ok=%v err=%v", ok, err)
	}
	if rec.SessionID != sess.ID || rec.AgentID != ag.ID {
		t.Fatalf("ids not stamped: %+v", rec)
	}
	if len(rec.Todos) != 2 || rec.Todos[0].Status != "completed" {
		t.Fatalf("todos not persisted: %+v", rec.Todos)
	}

	// A SECOND session on the SAME cwd resolves to a DIFFERENT progress dir.
	sess2, _ := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID, WorkingDir: cwd})
	if rt.ProgressDir(sess2.ID) == rt.ProgressDir(sess.ID) {
		t.Fatalf("two sessions on the same cwd share a progress dir: %s", rt.ProgressDir(sess.ID))
	}
}

// TestNewTodoSinkFallsBackPerSession verifies that with no explicit project dir
// the sink writes to a PER-SESSION file under the workspace store, so unrelated
// sessions don't collide on one shared progress file.
func TestNewTodoSinkFallsBackPerSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	ag, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	sess, _ := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID}) // no WorkingDir

	sink := rt.NewTodoSink(sess.ID, ag.ID)
	if err := sink.SaveTodos(ctx, []tools.TodoSinkItem{{Content: "x", Status: "pending"}}); err != nil {
		t.Fatalf("save todos: %v", err)
	}

	fallback := filepath.Join(rt.db.Root(), "progress", sess.ID)
	rec, ok, err := progress.Load(fallback)
	if err != nil || !ok {
		t.Fatalf("load fallback: ok=%v err=%v", ok, err)
	}
	if len(rec.Todos) != 1 || rec.Todos[0].Content != "x" {
		t.Fatalf("fallback todos wrong: %+v", rec.Todos)
	}

	// A SECOND session (same agent, no project dir) must get its OWN file, not
	// the first session's — this is the per-session isolation the fix provides.
	sess2, _ := rt.db.CreateSession(ctx, db.Session{AgentID: ag.ID})
	if rt.ProgressDir(sess2.ID) == rt.ProgressDir(sess.ID) {
		t.Fatalf("two sessions resolved to the SAME progress dir: %s", rt.ProgressDir(sess.ID))
	}
}

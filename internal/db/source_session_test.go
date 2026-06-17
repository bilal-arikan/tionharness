package db

import (
	"context"
	"testing"
)

// TestGetOrCreateSourceSession verifies the unification primitive: a (kind,
// sourceID) pair maps to exactly one session (idempotent), distinct source ids
// get distinct sessions, and the session round-trips its SourceID through reload.
func TestGetOrCreateSourceSession(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ctx := context.Background()

	s1, err := d.GetOrCreateSourceSession(ctx, "task", "task-1", "agent-1", "First task")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if s1.Kind != "task" || s1.SourceID != "task-1" {
		t.Fatalf("unexpected session fields: %+v", s1)
	}

	// Same (kind, sourceID) returns the same session.
	again, err := d.GetOrCreateSourceSession(ctx, "task", "task-1", "agent-1", "First task")
	if err != nil {
		t.Fatalf("re-get: %v", err)
	}
	if again.ID != s1.ID {
		t.Fatalf("expected same session, got %s vs %s", again.ID, s1.ID)
	}

	// Different sourceID → different session.
	s2, err := d.GetOrCreateSourceSession(ctx, "task", "task-2", "agent-1", "Second task")
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	if s2.ID == s1.ID {
		t.Fatalf("expected distinct sessions for distinct source ids")
	}

	// Same sourceID but different kind → different session.
	f1, err := d.GetOrCreateSourceSession(ctx, "flow", "task-1", "agent-1", "A flow")
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	if f1.ID == s1.ID {
		t.Fatalf("expected distinct sessions across kinds")
	}

	// Reload from disk and confirm SourceID survived.
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.GetSession(ctx, s1.ID)
	if err != nil {
		t.Fatalf("get after reload: %v", err)
	}
	if got.SourceID != "task-1" || got.Kind != "task" {
		t.Fatalf("source/kind lost on reload: %+v", got)
	}
}

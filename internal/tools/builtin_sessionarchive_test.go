package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// mkSession is a tiny helper that creates a session and returns its assigned id.
func mkSession(t *testing.T, database *db.DB, ctx context.Context, s db.Session) string {
	t.Helper()
	created, err := database.CreateSession(ctx, s)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return created.ID
}

// stateOf reads a session's current state (active/archived) for assertions.
func stateOf(t *testing.T, database *db.DB, ctx context.Context, id string) string {
	t.Helper()
	s, err := database.GetSession(ctx, id)
	if err != nil {
		t.Fatalf("get session %s: %v", id, err)
	}
	return s.State
}

// TestArchiveSessionsExcludesCurrentAndScope verifies the core safety guarantees:
// the current session and non-chat / already-archived sessions are never touched,
// and everything else active is archived.
func TestArchiveSessionsExcludesCurrentAndScope(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	current := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Current", State: "active"})
	alpha := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Alpha", State: "active"})
	beta := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Beta", State: "active"})
	arch := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Gamma", State: "archived"})
	sched := mkSession(t, database, ctx, db.Session{Kind: "schedule", Title: "Pulse", State: "active"})

	tool := NewArchiveSessionsTool(database, current)

	out, err := tool.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Archived 2 session") {
		t.Fatalf("expected 2 archived (Alpha+Beta), got:\n%s", out)
	}

	if got := stateOf(t, database, ctx, current); got != "active" {
		t.Fatalf("current session must stay active, got %q", got)
	}
	if got := stateOf(t, database, ctx, alpha); got != "archived" {
		t.Fatalf("alpha should be archived, got %q", got)
	}
	if got := stateOf(t, database, ctx, beta); got != "archived" {
		t.Fatalf("beta should be archived, got %q", got)
	}
	if got := stateOf(t, database, ctx, arch); got != "archived" {
		t.Fatalf("already-archived gamma should stay archived, got %q", got)
	}
	if got := stateOf(t, database, ctx, sched); got != "active" {
		t.Fatalf("non-chat schedule session must never be archived, got %q", got)
	}
}

// TestArchiveSessionsDryRun checks that dry_run reports matches but changes nothing.
func TestArchiveSessionsDryRun(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	current := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Current", State: "active"})
	alpha := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Alpha", State: "active"})

	tool := NewArchiveSessionsTool(database, current)
	out, err := tool.Call(ctx, json.RawMessage(`{"dry_run":true}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "DRY RUN") || !strings.Contains(out, "Alpha") {
		t.Fatalf("dry run should list Alpha:\n%s", out)
	}
	if got := stateOf(t, database, ctx, alpha); got != "active" {
		t.Fatalf("dry run must not change state, alpha is %q", got)
	}
}

// TestArchiveSessionsFilters checks title_contains and the idle_days age gate.
func TestArchiveSessionsFilters(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	current := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Current", State: "active"})
	keepMe := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Keep this", State: "active"})
	testOne := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "test run one", State: "active"})

	tool := NewArchiveSessionsTool(database, current)

	// title_contains: only "test run one" matches "test".
	out, err := tool.Call(ctx, json.RawMessage(`{"title_contains":"test"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Archived 1 session") {
		t.Fatalf("expected only 1 archived by title filter:\n%s", out)
	}
	if got := stateOf(t, database, ctx, keepMe); got != "active" {
		t.Fatalf("title filter should have kept 'Keep this' active, got %q", got)
	}
	if got := stateOf(t, database, ctx, testOne); got != "archived" {
		t.Fatalf("title-matched session should be archived, got %q", got)
	}

	// idle_days: all sessions were just created, so a large idle window matches none.
	out, err = tool.Call(ctx, json.RawMessage(`{"idle_days":9999}`))
	if err != nil {
		t.Fatalf("call idle: %v", err)
	}
	if !strings.Contains(out, "nothing to archive") {
		t.Fatalf("recent sessions should not match a 9999-day idle filter:\n%s", out)
	}
	if got := stateOf(t, database, ctx, keepMe); got != "active" {
		t.Fatalf("idle filter must have left 'Keep this' active, got %q", got)
	}
}

// TestArchiveSessionsIncludeCurrent verifies the current session is excluded by
// default but archivable when include_current:true is passed.
func TestArchiveSessionsIncludeCurrent(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	current := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Current", State: "active"})
	other := mkSession(t, database, ctx, db.Session{Kind: "chat", Title: "Other", State: "active"})

	tool := NewArchiveSessionsTool(database, current)

	// Default: only the OTHER session is archived; current stays active.
	out, err := tool.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, other) || strings.Contains(out, current) {
		t.Fatalf("default run must archive only the other session:\n%s", out)
	}
	if got := stateOf(t, database, ctx, current); got != "active" {
		t.Fatalf("current session must stay active by default, got %q", got)
	}

	// include_current: the current session is now archivable too.
	out, err = tool.Call(ctx, json.RawMessage(`{"include_current":true}`))
	if err != nil {
		t.Fatalf("call include_current: %v", err)
	}
	if !strings.Contains(out, current) {
		t.Fatalf("include_current must archive the current session:\n%s", out)
	}
	if got := stateOf(t, database, ctx, current); got != "archived" {
		t.Fatalf("current session should be archived, got %q", got)
	}
}

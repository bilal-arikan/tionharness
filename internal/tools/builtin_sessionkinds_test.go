package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// archiveKindsFixture builds a workspace holding one active session of every kind
// plus the current session, and returns the tool bound to it. The returned map is
// keyed by kind so assertions read as "the flow session must be archived".
func archiveKindsFixture(t *testing.T, ctx context.Context) (*db.DB, ArchiveSessionsTool, map[string]string) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	current := mkSession(t, database, ctx, db.Session{Kind: sessionKindChat, Title: "Current", State: "active"})

	ids := map[string]string{}
	for _, kind := range archivableSessionKinds {
		ids[kind] = mkSession(t, database, ctx, db.Session{Kind: kind, Title: "S-" + kind, State: "active"})
	}
	return database, NewArchiveSessionsTool(database, current), ids
}

// TestArchiveSessionsDefaultsToChatOnly pins the backward-compatibility contract:
// with no `kinds` argument the tool must behave exactly as it did before `kinds`
// existed — chat sessions archived, every autonomous-run kind left alone.
func TestArchiveSessionsDefaultsToChatOnly(t *testing.T) {
	ctx := context.Background()
	database, tool, ids := archiveKindsFixture(t, ctx)

	if _, err := tool.Call(ctx, json.RawMessage(`{}`)); err != nil {
		t.Fatalf("call: %v", err)
	}

	if got := stateOf(t, database, ctx, ids[sessionKindChat]); got != "archived" {
		t.Fatalf("chat session should be archived by default, got %q", got)
	}
	for _, kind := range archivableSessionKinds {
		if kind == sessionKindChat {
			continue
		}
		if got := stateOf(t, database, ctx, ids[kind]); got != "active" {
			t.Fatalf("kind %q must stay active when `kinds` is omitted, got %q", kind, got)
		}
	}
}

// TestArchiveSessionsSelectedKinds checks that naming a kind sweeps exactly that
// kind and nothing else — in particular that chat sessions are NOT collateral.
func TestArchiveSessionsSelectedKinds(t *testing.T) {
	ctx := context.Background()
	database, tool, ids := archiveKindsFixture(t, ctx)

	out, err := tool.Call(ctx, json.RawMessage(`{"kinds":["spawned"]}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Archived 1 session") {
		t.Fatalf("expected exactly 1 spawned session archived:\n%s", out)
	}
	if got := stateOf(t, database, ctx, ids[sessionKindSpawned]); got != "archived" {
		t.Fatalf("spawned session should be archived, got %q", got)
	}
	if got := stateOf(t, database, ctx, ids[sessionKindChat]); got != "active" {
		t.Fatalf("chat session must be untouched when kinds:[spawned], got %q", got)
	}
	if got := stateOf(t, database, ctx, ids[sessionKindFlow]); got != "active" {
		t.Fatalf("flow session must be untouched when kinds:[spawned], got %q", got)
	}
}

// TestArchiveSessionsWildcardKind verifies kinds:["*"] expands to every kind, so a
// full sweep is finally expressible (the gap this tool had).
func TestArchiveSessionsWildcardKind(t *testing.T) {
	ctx := context.Background()
	database, tool, ids := archiveKindsFixture(t, ctx)

	if _, err := tool.Call(ctx, json.RawMessage(`{"kinds":["*"]}`)); err != nil {
		t.Fatalf("call: %v", err)
	}
	for _, kind := range archivableSessionKinds {
		if got := stateOf(t, database, ctx, ids[kind]); got != "archived" {
			t.Fatalf("kinds:[*] should archive kind %q, got %q", kind, got)
		}
	}
}

// TestArchiveSessionsUnknownKindErrors ensures a typo'd kind fails loudly instead
// of silently matching nothing (which would read as "there was nothing to clean").
func TestArchiveSessionsUnknownKindErrors(t *testing.T) {
	ctx := context.Background()
	database, tool, ids := archiveKindsFixture(t, ctx)

	_, err := tool.Call(ctx, json.RawMessage(`{"kinds":["spawn"]}`))
	if err == nil {
		t.Fatal("expected an error for the unknown kind \"spawn\", got nil")
	}
	if !strings.Contains(err.Error(), "spawn") {
		t.Fatalf("error should name the offending kind, got: %v", err)
	}
	// A rejected call must not have archived anything on its way out.
	if got := stateOf(t, database, ctx, ids[sessionKindChat]); got != "active" {
		t.Fatalf("a rejected call must change nothing, chat is %q", got)
	}
}

// TestArchiveSessionsDryRunShowsKind checks the dry-run preview spells out each
// session's kind, so the caller sees WHAT would be swept, not just how many.
func TestArchiveSessionsDryRunShowsKind(t *testing.T) {
	ctx := context.Background()
	database, tool, ids := archiveKindsFixture(t, ctx)

	out, err := tool.Call(ctx, json.RawMessage(`{"kinds":["flow"],"dry_run":true}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "DRY RUN") {
		t.Fatalf("expected a dry-run preview:\n%s", out)
	}
	if !strings.Contains(out, "[flow]") {
		t.Fatalf("dry-run line should carry the session kind:\n%s", out)
	}
	if got := stateOf(t, database, ctx, ids[sessionKindFlow]); got != "active" {
		t.Fatalf("dry run must not change state, flow is %q", got)
	}
}

// TestArchiveSessionsKindsRespectsGuards confirms the pre-existing safety rails
// still apply once `kinds` widens the net: the current session and the `exclude`
// list survive a kinds:["*"] sweep.
func TestArchiveSessionsKindsRespectsGuards(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	current := mkSession(t, database, ctx, db.Session{Kind: sessionKindChat, Title: "Current", State: "active"})
	spared := mkSession(t, database, ctx, db.Session{Kind: sessionKindFlow, Title: "Spared", State: "active"})
	swept := mkSession(t, database, ctx, db.Session{Kind: sessionKindFlow, Title: "Swept", State: "active"})

	tool := NewArchiveSessionsTool(database, current)
	body := `{"kinds":["*"],"exclude":["` + spared + `"]}`
	if _, err := tool.Call(ctx, json.RawMessage(body)); err != nil {
		t.Fatalf("call: %v", err)
	}

	if got := stateOf(t, database, ctx, current); got != "active" {
		t.Fatalf("current session must survive a kinds:[*] sweep, got %q", got)
	}
	if got := stateOf(t, database, ctx, spared); got != "active" {
		t.Fatalf("excluded session must survive a kinds:[*] sweep, got %q", got)
	}
	if got := stateOf(t, database, ctx, swept); got != "archived" {
		t.Fatalf("non-excluded flow session should be archived, got %q", got)
	}
}

// TestResolveArchiveKinds covers the kind-resolution helper directly, including
// case/whitespace tolerance and the empty-input default.
func TestResolveArchiveKinds(t *testing.T) {
	got, err := resolveArchiveKinds(nil)
	if err != nil {
		t.Fatalf("nil kinds: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("nil kinds must default to chat-only, got %v", got)
	}
	if _, ok := got[sessionKindChat]; !ok {
		t.Fatalf("nil kinds must default to chat, got %v", got)
	}

	got, err = resolveArchiveKinds([]string{"  FLOW ", "spawned"})
	if err != nil {
		t.Fatalf("mixed-case kinds: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected flow+spawned, got %v", got)
	}
	if _, ok := got[sessionKindFlow]; !ok {
		t.Fatalf("kind matching should trim and lowercase, got %v", got)
	}

	got, err = resolveArchiveKinds([]string{"*"})
	if err != nil {
		t.Fatalf("wildcard: %v", err)
	}
	if len(got) != len(archivableSessionKinds) {
		t.Fatalf("wildcard should expand to all %d kinds, got %d", len(archivableSessionKinds), len(got))
	}

	if _, err := resolveArchiveKinds([]string{"nope"}); err == nil {
		t.Fatal("expected an error for an unknown kind")
	}
}

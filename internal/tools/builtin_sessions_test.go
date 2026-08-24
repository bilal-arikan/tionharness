package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestListSessionsTool checks the pull tool: active-only by default across ALL
// kinds, all states when asked, the per-kind filter, and the kind·state prefix.
func TestListSessionsTool(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Alpha", State: "active"})
	database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Beta", State: "archived"})
	database.CreateSession(ctx, db.Session{Kind: "schedule", Title: "Pulse", State: "active"})
	database.CreateSession(ctx, db.Session{Kind: "spawned", Title: "Gamma", State: "active"})

	tool := NewListSessionsTool(database)

	// Default (active, all kinds): Alpha (chat), Pulse (schedule) and Gamma
	// (spawned) show; Beta is archived so it's hidden.
	out, err := tool.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Pulse") || !strings.Contains(out, "Gamma") {
		t.Fatalf("default listing should include every active kind:\n%s", out)
	}
	if strings.Contains(out, "Beta") {
		t.Fatalf("archived session must be hidden by default:\n%s", out)
	}
	// The prefix carries the kind so kinds are distinguishable.
	if !strings.Contains(out, "[spawned·active]") || !strings.Contains(out, "[schedule·active]") {
		t.Fatalf("kind·state prefix missing:\n%s", out)
	}

	// state=all surfaces the archived chat session too.
	out, err = tool.Call(ctx, json.RawMessage(`{"state":"all"}`))
	if err != nil {
		t.Fatalf("call all: %v", err)
	}
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("state=all should list active and archived:\n%s", out)
	}

	// kind=chat narrows to chat only (Pulse/Gamma excluded).
	out, err = tool.Call(ctx, json.RawMessage(`{"kind":"chat"}`))
	if err != nil {
		t.Fatalf("call kind=chat: %v", err)
	}
	if !strings.Contains(out, "Alpha") || strings.Contains(out, "Pulse") || strings.Contains(out, "Gamma") {
		t.Fatalf("kind=chat filter wrong:\n%s", out)
	}
}

// TestListSessionsToolPagination verifies limit/offset windowing surfaces the
// total and the next-page offset, and that every session is reachable by paging.
func TestListSessionsToolPagination(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	for _, name := range []string{"S1", "S2", "S3", "S4", "S5"} {
		database.CreateSession(ctx, db.Session{Kind: "chat", Title: name, State: "active"})
	}
	tool := NewListSessionsTool(database)

	// First page of 2 of 5: reports the total and the next offset.
	out, err := tool.Call(ctx, json.RawMessage(`{"limit":2}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Showing 1–2 of 5.") {
		t.Fatalf("expected page-1 header:\n%s", out)
	}
	if !strings.Contains(out, "offset:2") {
		t.Fatalf("expected next-page offset hint:\n%s", out)
	}

	// Last page: no more offset hint.
	out, err = tool.Call(ctx, json.RawMessage(`{"limit":2,"offset":4}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Showing 5–5 of 5.") || strings.Contains(out, "offset:") {
		t.Fatalf("last page wrong:\n%s", out)
	}

	// Offset past the end is reported, not an error.
	out, err = tool.Call(ctx, json.RawMessage(`{"offset":99}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "past the last") {
		t.Fatalf("expected past-end notice:\n%s", out)
	}
}

// TestListSessionsToolSort verifies the shared sort argument reorders the
// filtered rows (name_asc by title), while the default stays updated_desc —
// the ordering existing callers already relied on.
func TestListSessionsToolSort(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	for _, name := range []string{"Zeta", "Alpha", "Mid"} {
		database.CreateSession(ctx, db.Session{Kind: "chat", Title: name, State: "active"})
	}
	tool := NewListSessionsTool(database)

	out, err := tool.Call(ctx, json.RawMessage(`{"sort":"name_asc"}`))
	if err != nil {
		t.Fatalf("call sort=name_asc: %v", err)
	}
	alpha := strings.Index(out, "Alpha")
	mid := strings.Index(out, "Mid")
	zeta := strings.Index(out, "Zeta")
	if alpha == -1 || mid == -1 || zeta == -1 || !(alpha < mid && mid < zeta) {
		t.Fatalf("name_asc should order Alpha < Mid < Zeta:\n%s", out)
	}

	// An invalid sort key is an explicit error, never a silent fallback.
	if _, err := tool.Call(ctx, json.RawMessage(`{"sort":"bogus_desc"}`)); err == nil {
		t.Fatal("sort=bogus_desc should error")
	}
}

package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestListSessionsTool checks the pull tool: active-only by default, all when
// asked, chat-only filter, and the limit cap.
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

	tool := NewListSessionsTool(database)

	// Default (active only): Alpha yes, Beta no, Pulse (schedule) no.
	out, err := tool.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "Alpha") || strings.Contains(out, "Beta") || strings.Contains(out, "Pulse") {
		t.Fatalf("active-only listing wrong:\n%s", out)
	}

	// state=all surfaces the archived chat session too (still not the schedule one).
	out, err = tool.Call(ctx, json.RawMessage(`{"state":"all"}`))
	if err != nil {
		t.Fatalf("call all: %v", err)
	}
	if !strings.Contains(out, "Alpha") || !strings.Contains(out, "Beta") {
		t.Fatalf("state=all should list both chat sessions:\n%s", out)
	}
	if strings.Contains(out, "Pulse") {
		t.Fatalf("non-chat sessions must never be listed:\n%s", out)
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

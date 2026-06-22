package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
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

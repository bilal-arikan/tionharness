package api

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestSessionsContextBlock verifies the cross-session block: active vs recent
// split, current-session exclusion, the chat-only filter, and graceful empty.
func TestSessionsContextBlock(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	current, _ := database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Current Work", State: "active"})
	database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Other Active", State: "active", Summary: "wiring the API"})
	database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Old Thread", State: "archived"})
	database.CreateSession(ctx, db.Session{Kind: "schedule", Title: "Pulse Loop", State: "active"})

	block := sessionsContextBlock(ctx, database, current.ID, 5)
	if block == "" {
		t.Fatal("expected a non-empty block")
	}
	if !strings.Contains(block, "Other Active") {
		t.Errorf("active session missing:\n%s", block)
	}
	if !strings.Contains(block, "wiring the API") {
		t.Errorf("summary snippet missing:\n%s", block)
	}
	if !strings.Contains(block, "Old Thread") {
		t.Errorf("recent (archived) session missing:\n%s", block)
	}
	if strings.Contains(block, "Current Work") {
		t.Errorf("current session must be excluded:\n%s", block)
	}
	if strings.Contains(block, "Pulse Loop") {
		t.Errorf("non-chat (schedule) session must be filtered out:\n%s", block)
	}
	if !strings.Contains(block, "Active:") || !strings.Contains(block, "Recent:") {
		t.Errorf("expected both Active and Recent sections:\n%s", block)
	}
}

// TestSessionsContextBlockEmpty returns "" when only the current session exists.
func TestSessionsContextBlockEmpty(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	current, _ := database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Only One", State: "active"})
	if block := sessionsContextBlock(ctx, database, current.ID, 5); block != "" {
		t.Fatalf("expected empty block, got:\n%s", block)
	}
}

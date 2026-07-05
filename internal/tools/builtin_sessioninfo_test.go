package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestGetSessionInfoTool covers the three lookup paths: explicit session_id,
// the ctx-injected current session, and the graceful no-session message.
func TestGetSessionInfoTool(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	sess, err := database.CreateSession(ctx, db.Session{
		Kind: "chat", Title: "Refactor sweep", State: "active",
		Tags: []string{"sprint", "backend"}, Goal: "ship it", Role: "coordinator",
		WorkingDir: `C:\proj`,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	tool := NewGetSessionInfoTool(database)

	// Explicit session_id.
	out, err := tool.Call(ctx, json.RawMessage(`{"session_id":"`+sess.ID+`"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	for _, want := range []string{sess.ID, "Refactor sweep", "sprint, backend", "ship it", "coordinator", `C:\proj`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}

	// Default: the ctx-injected current session.
	out, err = tool.Call(WithCurrentSession(ctx, sess.ID), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call via ctx: %v", err)
	}
	if !strings.Contains(out, "Refactor sweep") {
		t.Fatalf("ctx-session lookup failed:\n%s", out)
	}

	// No session anywhere: graceful message, not an error.
	out, err = tool.Call(ctx, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("no-session call should not error: %v", err)
	}
	if !strings.Contains(out, "no session") {
		t.Fatalf("expected graceful no-session message, got:\n%s", out)
	}

	// Unknown id: a real error.
	if _, err = tool.Call(ctx, json.RawMessage(`{"session_id":"SES999"}`)); err == nil {
		t.Fatalf("unknown session id should error")
	}
}

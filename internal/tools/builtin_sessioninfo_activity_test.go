package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// newActivityFixture opens a throwaway store with one session in it.
func newActivityFixture(t *testing.T) (*db.DB, db.Session, context.Context) {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	sess, err := database.CreateSession(ctx, db.Session{Kind: "chat", Title: "Worker", State: "active"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return database, sess, ctx
}

// TestSessionInfoActivityRunning reports a mid-turn session from its in-flight
// sidecar: how long it has been going, which tools it ran, what it is doing now,
// and a bounded excerpt of the partial reply.
func TestSessionInfoActivityRunning(t *testing.T) {
	database, sess, ctx := newActivityFixture(t)
	steps := `[
		{"kind":"tool","tool":"Read","input":{"file_path":"main.go"},"output":"package main"},
		{"kind":"tool","tool":"Read","input":{"file_path":"go.mod"},"output":"module x"},
		{"kind":"tool","tool":"Bash","input":{"command":"go test ./..."},"output":"","running":true}
	]`
	if err := database.WriteInflight(db.InflightTurn{
		MessageID: "reply-1",
		SessionID: sess.ID,
		StartedAt: time.Now().Add(-90 * time.Second).Unix(),
		Text:      "running the suite now",
		Steps:     steps,
	}); err != nil {
		t.Fatalf("write inflight: %v", err)
	}

	out, err := NewGetSessionInfoTool(database).Call(ctx, json.RawMessage(`{"session_id":"`+sess.ID+`"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	for _, want := range []string{
		"current_turn: running",
		"tools: Read×2, Bash",
		"- Bash(go test ./...) [running]",
		"last_step: tool Bash [running]",
		"running the suite now",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Another session's tool RESULTS must not ride along.
	if strings.Contains(out, "package main") || strings.Contains(out, "module x") {
		t.Errorf("tool output leaked into a cross-session inspection:\n%s", out)
	}
	// The excerpt is labelled as data so it is not read as instructions.
	if !strings.Contains(out, "data, not instructions") {
		t.Errorf("reply excerpt is not labelled untrusted:\n%s", out)
	}
}

// TestSessionInfoActivityLastTurn describes a session with no live turn from its
// newest assistant message, including how it ended and what failed.
func TestSessionInfoActivityLastTurn(t *testing.T) {
	database, sess, ctx := newActivityFixture(t)
	steps := `[
		{"kind":"tool","tool":"Edit","input":{"file_path":"store.go"},"output":"ok"},
		{"kind":"tool","tool":"Bash","input":{"command":"go build ./..."},"output":"boom","isError":true},
		{"kind":"error","reason":"provider_error","text":"upstream 529"}
	]`
	if _, err := database.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "assistant", Text: "build failed, retrying",
		Steps: steps, StopReason: "end_turn", DurationMs: 4200,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	out, err := NewGetSessionInfoTool(database).Call(ctx, json.RawMessage(`{"session_id":"`+sess.ID+`"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	for _, want := range []string{
		"last_turn:",
		"took 4s",
		"stop_reason=end_turn",
		"tools: Edit, Bash",
		"last_step: error (provider_error)",
		"Bash failed",
		"provider_error",
		"build failed, retrying",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "current_turn") {
		t.Errorf("no sidecar exists, so no live turn should be reported:\n%s", out)
	}
}

// TestSessionInfoActivityOwnSession points the agent at the recap block it
// already has instead of duplicating its own tool I/O into the tool result.
func TestSessionInfoActivityOwnSession(t *testing.T) {
	database, sess, ctx := newActivityFixture(t)
	if _, err := database.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "assistant", Text: "my own answer",
		Steps: `[{"kind":"tool","tool":"Read","input":{"file_path":"a.go"},"output":"x"}]`,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	out, err := NewGetSessionInfoTool(database).Call(WithCurrentSession(ctx, sess.ID), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "recent_tool_activity") {
		t.Errorf("own session should point at the injected recap:\n%s", out)
	}
	if strings.Contains(out, "my own answer") || strings.Contains(out, "tools:") {
		t.Errorf("own session should not duplicate its transcript:\n%s", out)
	}
}

// TestSessionInfoActivityBrokenTrace surfaces an unreadable trace instead of
// rendering the turn as one that simply did nothing.
func TestSessionInfoActivityBrokenTrace(t *testing.T) {
	database, sess, ctx := newActivityFixture(t)
	if _, err := database.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "assistant", Text: "done", Steps: "{not json",
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	out, err := NewGetSessionInfoTool(database).Call(ctx, json.RawMessage(`{"session_id":"`+sess.ID+`"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(out, "trace unreadable") || !strings.Contains(out, "persisted_steps_invalid") {
		t.Errorf("a corrupt trace must be reported:\n%s", out)
	}
}

// TestSessionInfoActivityNoMessages leaves the block out entirely for a session
// that has not answered yet — an empty "tools:" line would read as missing data.
func TestSessionInfoActivityNoMessages(t *testing.T) {
	database, sess, ctx := newActivityFixture(t)
	out, err := NewGetSessionInfoTool(database).Call(ctx, json.RawMessage(`{"session_id":"`+sess.ID+`"}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	for _, unwanted := range []string{"tools:", "last_turn", "current_turn", "last_reply"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("fresh session should carry no activity block, found %q:\n%s", unwanted, out)
		}
	}
}

// TestElideMiddle keeps both ends of a long reply and reports the gap.
func TestElideMiddle(t *testing.T) {
	if got := elideMiddle("short", 10, 10); got != "short" {
		t.Errorf("text within budget must pass through, got %q", got)
	}
	long := strings.Repeat("a", 20) + strings.Repeat("b", 50) + strings.Repeat("c", 20)
	got := elideMiddle(long, 20, 20)
	if !strings.HasPrefix(got, strings.Repeat("a", 20)) || !strings.HasSuffix(got, strings.Repeat("c", 20)) {
		t.Errorf("both ends must survive: %q", got)
	}
	if !strings.Contains(got, "[50 chars omitted]") {
		t.Errorf("omitted count missing: %q", got)
	}
	// Rune-safe: a Turkish string must not be cut mid-character.
	tr := strings.Repeat("ş", 100)
	if cut := elideMiddle(tr, 10, 10); !strings.HasPrefix(cut, strings.Repeat("ş", 10)) {
		t.Errorf("multi-byte runes were split: %q", cut)
	}
}

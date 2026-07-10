package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// TestPreToolHookDebugEventCarriesHookID verifies a fired PreToolUse hook writes a
// DebugHook journal event stamped with the firing hook's id — the attribution the
// HookActivity viz relies on to split rtk/sqz activity per hook.
func TestPreToolHookDebugEventCarriesHookID(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	sess, err := rt.db.CreateSession(ctx, db.Session{Kind: "chat", AgentID: "AGT1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	h, err := rt.db.CreateHook(ctx, db.Hook{
		Event: db.HookPreToolUse, Type: "command", Matcher: "Bash",
		Command: "echo ok", Enabled: true, TimeoutSec: 10, // harmless no-op; classification is by command elsewhere
	})
	if err != nil {
		t.Fatalf("create hook: %v", err)
	}

	sctx := WithSessionID(ctx, sess.ID)
	rt.runPreToolHooks(sctx, sess.ID, providers.ToolCall{ID: "c1", Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)})

	evs, err := rt.db.ReadDebugEvents(ctx, sess.ID, db.DebugHook, 0)
	if err != nil {
		t.Fatalf("read debug: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("expected exactly one hook debug event, got %d: %+v", len(evs), evs)
	}
	if evs[0].HookID != h.ID {
		t.Errorf("debug event HookID = %q, want %q", evs[0].HookID, h.ID)
	}
	if evs[0].Name != db.HookPreToolUse {
		t.Errorf("debug event Name = %q, want %q", evs[0].Name, db.HookPreToolUse)
	}
}

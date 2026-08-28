package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestGatewayActivationSurvivesNewToken is the SES79 regression: the activated
// extended set is persisted per SESSION, so a claude-cli subprocess reconnecting
// under a fresh Bearer token (or the process restarting) still sees the tools it
// activated. Before this, the set lived only in RAM keyed by token and every
// call answered "No such tool available: mcp__tionharness_extended__<name>".
func TestGatewayActivationSurvivesNewToken(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	sess, err := database.CreateSession(context.Background(), db.Session{})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	runs := newChatRuns()
	b := &interactionBackend{
		runs: runs,
		tun:  agent.NewTunables(),
		activationStoreFor: func(run *chatRun) (*db.DB, string) {
			return database, run.sessionID
		},
	}

	// Turn 1: activate an extended tool.
	run1 := runs.register("r1", sess.ID, "ws1", func() {})
	tok1 := "token-turn-1"
	runs.bindActive(tok1, run1)
	res, err := b.callActivate(tok1, run1, json.RawMessage(`{"tools":["notify"]}`), true)
	if err != nil || res.IsError {
		t.Fatalf("activate: err=%v res=%q", err, res.Text)
	}
	names, ok, err := database.ReadActivatedTools(sess.ID)
	if err != nil || !ok || len(names) != 1 || names[0] != "notify" {
		t.Fatalf("sidecar after activate = (%v, %v, %v), want [notify]", names, ok, err)
	}

	// Turn 2: the SAME session reconnects under a brand-new token, and a fresh
	// backend (the process restarted — nothing left in RAM).
	runs2 := newChatRuns()
	b2 := &interactionBackend{
		runs: runs2,
		tun:  agent.NewTunables(),
		activationStoreFor: func(run *chatRun) (*db.DB, string) {
			return database, run.sessionID
		},
	}
	run2 := runs2.register("r2", sess.ID, "ws1", func() {})
	tok2 := "token-turn-2"
	runs2.bindActive(tok2, run2)

	if !b2.isActivated(tok2, "notify") {
		t.Fatal("notify must still be activated for the same session under a new token")
	}
	if ext := b2.Tools(tok2, "extended"); !specHasTool(ext, "notify") {
		t.Fatalf("extended tier must re-advertise notify after rehydrate, got %v", specNames(ext))
	}
	if res := b2.callActiveTools(tok2); !strings.Contains(res.Text, "notify") {
		t.Fatalf("active_tools must list the rehydrated tool, got %q", res.Text)
	}

	// Deactivating on the new token clears the durable copy too.
	if _, err := b2.callActivate(tok2, run2, json.RawMessage(`{"tools":["notify"]}`), false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, ok, _ := database.ReadActivatedTools(sess.ID); ok {
		t.Fatal("deactivating the last tool must clear the sidecar")
	}
}

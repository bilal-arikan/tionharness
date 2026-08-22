package db

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readSessionHeaderFile decodes the raw session.json header straight off disk,
// bypassing Open()'s reconcile-on-load recompute. Anything that reads the store
// as files (counter automations, external tooling) sees exactly this.
func readSessionHeaderFile(t *testing.T, storeDir, sessionID string) Session {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(storeDir, dirSessions, sessionID, sessionHeaderFile))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode header: %v", err)
	}
	return s
}

// TestHeaderCountersStaleAfterAppendUntilNextHeaderWrite documents the store's
// actual contract, which is easy to trip over and was tripped over by the
// insight scan flow:
//
//   - AddMessage appends ONE transcript line (O(1)) and deliberately does NOT
//     rewrite the header, so MessageCount/ToolCallCount/UpdatedAt on disk stay at
//     whatever the last header write left there.
//   - Any full header write (a title/state/summary change) picks the fresh
//     counters up.
//
// Consequence: write metadata AFTER the message, not before. Doing it before
// leaves "messageCount": 0 in session.json until the session is next mutated.
// The in-memory store is correct either way, and a reopened store recomputes the
// counters from the message lines — but the FILE is what external readers see.
func TestHeaderCountersStaleAfterAppendUntilNextHeaderWrite(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: "insight"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	steps := `[{"kind":"tool","name":"read"},{"kind":"tool","name":"grep"}]`
	wantTools := countToolSteps(steps)
	if wantTools == 0 {
		t.Fatalf("test fixture broken: countToolSteps(%s) = 0", steps)
	}
	if _, err := d.AddMessage(ctx, Message{
		SessionID: sess.ID,
		Role:      "assistant",
		Text:      "report",
		Steps:     steps,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	// The append alone leaves the header behind — this is the hot path's price.
	if hdr := readSessionHeaderFile(t, storeDir, sess.ID); hdr.MessageCount != 0 {
		t.Fatalf("append must not rewrite the header: on-disk MessageCount = %d, want 0", hdr.MessageCount)
	}

	// A title change is a full header write, so it carries the counters along.
	if err := d.SetSessionTitle(ctx, sess.ID, "scan result"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	hdr := readSessionHeaderFile(t, storeDir, sess.ID)
	if hdr.MessageCount != 1 {
		t.Fatalf("on-disk MessageCount after header write = %d, want 1", hdr.MessageCount)
	}
	if hdr.ToolCallCount != wantTools {
		t.Fatalf("on-disk ToolCallCount = %d, want %d", hdr.ToolCallCount, wantTools)
	}
	if hdr.Title != "scan result" {
		t.Fatalf("on-disk Title = %q, want %q", hdr.Title, "scan result")
	}
	if hdr.UpdatedAt < hdr.CreatedAt {
		t.Fatalf("on-disk UpdatedAt = %d, older than CreatedAt %d", hdr.UpdatedAt, hdr.CreatedAt)
	}
	_ = d.Close()

	// A reopened store recomputes from the message lines regardless of the header.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	got, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.MessageCount != 1 || got.ToolCallCount != wantTools {
		t.Fatalf("reloaded MessageCount/ToolCallCount = %d/%d, want 1/%d",
			got.MessageCount, got.ToolCallCount, wantTools)
	}
}

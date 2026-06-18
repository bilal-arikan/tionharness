package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRecoverInflightMaterialises verifies that an orphaned sidecar (a turn that
// was streaming when the process died) is reconstructed as an interrupted
// assistant message on the next open, and the sidecar is then removed.
func TestRecoverInflightMaterialises(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	// The user message was persisted before streaming began (as in the real flow).
	d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "hi"})
	// Simulate a crash mid-stream: a sidecar exists but the reply was never added.
	if err := d.WriteInflight(InflightTurn{
		MessageID: "reply-1",
		SessionID: sess.ID,
		AgentID:   agent.ID,
		Text:      "partial ans",
		Steps:     `[{"kind":"text","text":"partial ans"}]`,
	}); err != nil {
		t.Fatalf("write inflight: %v", err)
	}
	_ = d.Close()

	// Reopen → recovery should run during load().
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()

	msgs, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs) != 2 {
		t.Fatalf("expected user + recovered reply (2 msgs), got %d: %v", len(msgs), msgs)
	}
	got := msgs[1]
	if got.ID != "reply-1" || got.Role != "assistant" || got.Text != "partial ans" || !got.Interrupted {
		t.Fatalf("recovered message wrong: %+v", got)
	}
	// The sidecar must be gone after a successful recovery.
	if _, err := os.Stat(d2.inflightPath(sess.ID)); !os.IsNotExist(err) {
		t.Fatalf("sidecar should be removed after recovery, stat err=%v", err)
	}

	// Idempotent: reopening again must NOT duplicate the recovered reply.
	_ = d2.Close()
	d3, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen 2: %v", err)
	}
	defer d3.Close()
	msgs3, _ := d3.ListMessages(ctx, sess.ID)
	if len(msgs3) != 2 {
		t.Fatalf("recovery not idempotent, got %d msgs", len(msgs3))
	}
}

// TestRecoverInflightSkipsPersisted verifies that when the reply was already
// appended (crash in the tiny window before clearing the sidecar), recovery
// drops the sidecar without creating a duplicate message.
func TestRecoverInflightSkipsPersisted(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	// Reply already persisted with a known id...
	reply, _ := d.AddMessage(ctx, Message{ID: "reply-x", SessionID: sess.ID, Role: "assistant", Text: "done"})
	// ...but the sidecar for the same turn was never cleared (crash after append).
	d.WriteInflight(InflightTurn{MessageID: reply.ID, SessionID: sess.ID, AgentID: agent.ID, Text: "do"})
	_ = d.Close()

	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	msgs, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs) != 1 {
		t.Fatalf("expected no duplicate (1 msg), got %d: %v", len(msgs), msgs)
	}
	if msgs[0].Interrupted {
		t.Fatalf("already-persisted reply must not be flagged interrupted")
	}
	if _, err := os.Stat(d2.inflightPath(sess.ID)); !os.IsNotExist(err) {
		t.Fatalf("sidecar should be removed, stat err=%v", err)
	}
}

package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDeleteMessage verifies a single message is removed from memory and disk,
// the count is decremented, and the deletion survives a reopen.
func TestDeleteMessage(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	var ids []string
	for i := 0; i < 3; i++ {
		m, _ := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "m"})
		ids = append(ids, m.ID)
	}

	// Delete the middle message.
	if err := d.DeleteMessage(ctx, sess.ID, ids[1]); err != nil {
		t.Fatalf("delete: %v", err)
	}
	msgs, _ := d.ListMessages(ctx, sess.ID)
	if len(msgs) != 2 || msgs[0].ID != ids[0] || msgs[1].ID != ids[2] {
		t.Fatalf("after delete got %d msgs %v", len(msgs), msgs)
	}

	// Unknown message id → ErrNotFound.
	if err := d.DeleteMessage(ctx, sess.ID, "nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for unknown id, got %v", err)
	}
	_ = d.Close()

	// Reopen: the deletion must be persisted (2 messages remain, in order).
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	msgs2, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs2) != 2 || msgs2[0].ID != ids[0] || msgs2[1].ID != ids[2] {
		t.Fatalf("after reopen got %d msgs %v", len(msgs2), msgs2)
	}
}

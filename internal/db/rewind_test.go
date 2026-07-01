package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDeleteMessagesFrom verifies a conversation rewind: the anchor message and
// everything after it are removed, the count reflects the survivors, a stale
// rolling summary is reset, and the truncation survives a reopen.
func TestDeleteMessagesFrom(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	var ids []string
	for i := 0; i < 5; i++ {
		m, _ := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "m"})
		ids = append(ids, m.ID)
	}

	// Pretend the first 4 messages were folded into a rolling summary; rewinding
	// before that boundary must clear the now-stale summary.
	if err := d.SetSessionSummary(ctx, sess.ID, "old summary", 4); err != nil {
		t.Fatalf("set summary: %v", err)
	}

	// Rewind to the 3rd message (index 2): removes ids[2..4] → 3 removed, 2 left.
	removed, err := d.DeleteMessagesFrom(ctx, sess.ID, ids[2])
	if err != nil {
		t.Fatalf("rewind: %v", err)
	}
	if removed != 3 {
		t.Fatalf("expected 3 removed, got %d", removed)
	}
	msgs, _ := d.ListMessages(ctx, sess.ID)
	if len(msgs) != 2 || msgs[0].ID != ids[0] || msgs[1].ID != ids[1] {
		t.Fatalf("after rewind got %d msgs %v", len(msgs), msgs)
	}
	s2, _ := d.GetSession(ctx, sess.ID)
	if s2.Summary != "" || s2.SummaryMsgCount != 0 {
		t.Fatalf("stale summary not reset: %q count=%d", s2.Summary, s2.SummaryMsgCount)
	}
	if s2.MessageCount != 2 {
		t.Fatalf("expected MessageCount 2, got %d", s2.MessageCount)
	}

	// Unknown message id → ErrNotFound.
	if _, err := d.DeleteMessagesFrom(ctx, sess.ID, "nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	_ = d.Close()

	// Reopen: the truncation must be persisted.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	msgs2, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs2) != 2 {
		t.Fatalf("after reopen got %d msgs", len(msgs2))
	}
}

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestRoundTrip exercises the core entities and verifies that data survives a
// close + reopen cycle (i.e. it is genuinely persisted to disk, not just held
// in memory).
func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	storeDir := filepath.Join(root, "store")

	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	agent, err := d.CreateAgent(ctx, Agent{Name: "Tester", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T1"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "merhaba"}); err != nil {
		t.Fatalf("add msg1: %v", err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: "selam <b>html</b>"}); err != nil {
		t.Fatalf("add msg2: %v", err)
	}

	task, err := d.CreateTask(ctx, Task{Title: "Build", OwnerAgentID: agent.ID})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := d.AddUsage(ctx, agent.ID, 1, 100, 50); err != nil {
		t.Fatalf("add usage: %v", err)
	}

	// The session header must exist on disk.
	if _, err := os.Stat(filepath.Join(storeDir, dirSessions, sess.ID, sessionHeaderFile)); err != nil {
		t.Fatalf("session header missing: %v", err)
	}
	_ = d.Close()

	// Reopen and verify everything was reloaded from disk.
	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()

	if _, err := d2.GetAgent(ctx, agent.ID); err != nil {
		t.Fatalf("agent not reloaded: %v", err)
	}
	got, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("session not reloaded: %v", err)
	}
	if got.MessageCount != 2 {
		t.Fatalf("message_count = %d, want 2", got.MessageCount)
	}
	msgs, err := d2.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Text != "merhaba" || msgs[1].Text != "selam <b>html</b>" {
		t.Fatalf("messages wrong: %+v", msgs)
	}
	if _, err := d2.GetTask(ctx, task.ID); err != nil {
		t.Fatalf("task not reloaded: %v", err)
	}
	u, err := d2.GetUsageToday(ctx, agent.ID)
	if err != nil || u.Calls != 1 || u.InputTokens != 100 {
		t.Fatalf("usage wrong: %+v err=%v", u, err)
	}

	// Deletion must remove the file and the in-memory entry.
	if err := d2.DeleteTask(ctx, task.ID); err != nil {
		t.Fatalf("delete task: %v", err)
	}
	if _, err := d2.GetTask(ctx, task.ID); err != ErrNotFound {
		t.Fatalf("task still present after delete: %v", err)
	}
}

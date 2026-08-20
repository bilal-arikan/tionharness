package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestWorkerTerminalWritePersistsSharedCounters(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "Worker", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Kind: "worker"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "work"}); err != nil {
		t.Fatalf("add prompt: %v", err)
	}
	steps := `[{"kind":"tool","tool":"one"},{"kind":"tool","tool":"two"}]`
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: "done", Steps: steps}); err != nil {
		t.Fatalf("add reply: %v", err)
	}
	if err := d.SetSessionRunState(ctx, sess.ID, "completed", 1700000000); err != nil {
		t.Fatalf("set terminal state: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	reloaded, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reloaded.Close() })
	got, err := reloaded.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get reloaded session: %v", err)
	}
	if got.MessageCount != 2 || got.ToolCallCount != 2 {
		t.Fatalf("persisted counters = messages %d tools %d, want 2/2", got.MessageCount, got.ToolCallCount)
	}
}

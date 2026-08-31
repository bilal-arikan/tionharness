package db

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// TestAddMessageRollsBackWhenAppendFails: a message whose line never reached the
// transcript must not survive in memory. Otherwise the UI shows it, automations
// fire for it, and it vanishes on the next restart (the counters are recomputed
// from the file) — a silent data loss with no error anywhere the user can see.
func TestAddMessageRollsBackWhenAppendFails(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "ok"}); err != nil {
		t.Fatalf("first append: %v", err)
	}

	before, err := d.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	// Make the transcript file unwritable by removing the session directory: the
	// next append fails at os.OpenFile.
	if err := os.RemoveAll(d.dir(dirSessions, sess.ID)); err != nil {
		t.Fatalf("remove session dir: %v", err)
	}

	if _, err := d.AddMessage(ctx, Message{
		SessionID: sess.ID,
		Role:      "assistant",
		Text:      "lost",
		Steps:     `[{"type":"tool","tool":"Bash"}]`,
	}); err == nil {
		t.Fatal("AddMessage must return the append error")
	}

	after, err := d.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session after failure: %v", err)
	}
	if after.MessageCount != before.MessageCount {
		t.Fatalf("MessageCount = %d, want %d (rolled back)", after.MessageCount, before.MessageCount)
	}
	if after.ToolCallCount != before.ToolCallCount {
		t.Fatalf("ToolCallCount = %d, want %d (rolled back)", after.ToolCallCount, before.ToolCallCount)
	}
	if after.Unread != before.Unread {
		t.Fatalf("Unread = %v, want %v (rolled back)", after.Unread, before.Unread)
	}
	msgs, _, err := d.ListMessagesTail(ctx, sess.ID, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != before.MessageCount {
		t.Fatalf("in-memory transcript has %d messages, want %d", len(msgs), before.MessageCount)
	}
	for _, m := range msgs {
		if m.Text == "lost" {
			t.Fatal("the un-persisted message is still in memory")
		}
	}
}

// TestLoadJSONDirSkipsCorruptFile: one unparseable entity file must cost exactly
// that entity, not the whole store (and, one level up, not the whole workspace).
func TestLoadJSONDirSkipsCorruptFile(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 40; i++ {
		body := `{"id":"AGT` + strconv.Itoa(i) + `","name":"a"}`
		if i == 25 {
			body = `{"id": THIS IS NOT JSON`
		}
		if err := os.WriteFile(filepath.Join(dir, "AGT"+strconv.Itoa(i)+".json"), []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	got, err := loadJSONDir[Agent](dir)
	if err != nil {
		t.Fatalf("a corrupt entity file must not fail the load: %v", err)
	}
	if len(got) != 39 {
		t.Fatalf("loaded %d agents, want 39 (the corrupt one skipped)", len(got))
	}
	for _, a := range got {
		if a.ID == "AGT25" {
			t.Fatal("the corrupt file was loaded")
		}
	}
}

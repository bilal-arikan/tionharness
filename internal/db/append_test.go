package db

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAddMessageAppendsLine verifies the O(1) append hot-path: the session file
// is header line + one line per message (not rewritten), and reopening recovers
// every message in order.
func TestAddMessageAppendsLine(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID, Title: "T"})

	const n = 25
	for i := 0; i < n; i++ {
		if _, err := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "m"}); err != nil {
			t.Fatalf("add msg %d: %v", i, err)
		}
	}
	_ = d.Close()

	path := filepath.Join(storeDir, dirSessions, sess.ID, "session.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	// header line + n message lines = n+1 non-empty lines.
	lines := nonEmptyLines(string(raw))
	if len(lines) != n+1 {
		t.Fatalf("file has %d lines, want %d (header + %d msgs)", len(lines), n+1, n)
	}

	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer d2.Close()
	got, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.MessageCount != n {
		t.Fatalf("MessageCount = %d, want %d (recomputed from lines)", got.MessageCount, n)
	}
	msgs, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs) != n {
		t.Fatalf("reloaded %d messages, want %d", len(msgs), n)
	}
}

// TestReadSessionFileToleratesTornTrailingLine simulates a crash mid-append
// (a partial last line) and verifies load drops only that line, keeping the
// committed messages.
func TestReadSessionFileToleratesTornTrailingLine(t *testing.T) {
	ctx := context.Background()
	storeDir := filepath.Join(t.TempDir(), "store")
	d, err := Open(storeDir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	agent, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: agent.ID})
	_, _ = d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", Text: "one"})
	_, _ = d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", Text: "two"})
	_ = d.Close()

	// Append a torn (incomplete) JSON line, as a crash mid-write would leave.
	path := filepath.Join(storeDir, dirSessions, sess.ID, "session.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open for append: %v", err)
	}
	_, _ = f.WriteString(`{"id":"x","role":"user","text":"thr`) // no closing brace/newline
	_ = f.Close()

	d2, err := Open(storeDir)
	if err != nil {
		t.Fatalf("reopen should tolerate torn line, got: %v", err)
	}
	defer d2.Close()
	msgs, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs) != 2 {
		t.Fatalf("reloaded %d messages, want 2 (torn line dropped)", len(msgs))
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

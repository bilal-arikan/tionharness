package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestRecordSystemAgentSession: the exchange lands as a writable chat session
// owned by the workspace agent serving the role, with the user prompt and the
// agent reply as two ordinary messages and the role tag on the session.
func TestRecordSystemAgentSession(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	editor, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Goal Writer", System: true, SystemKey: "goal-writer", Soul: "write"})
	if err != nil {
		t.Fatal(err)
	}
	caller := db.Agent{ID: "AGT-caller", Provider: "claude-cli"}
	long := strings.Repeat("x", 200)
	id := rt.recordSystemAgentSession(ctx, "goal-writer", caller, "Hedef: "+long, "PROMPT", "REPLY")
	if id == "" {
		t.Fatal("expected a session id")
	}
	sess, err := rt.db.GetSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if sess.AgentID != editor.ID || sess.Kind != "chat" || !db.IsWritableSessionKind(sess.Kind) {
		t.Fatalf("session must be a writable chat owned by the role agent: %+v", sess)
	}
	if len(sess.Tags) != 1 || sess.Tags[0] != "system:goal-writer" {
		t.Fatalf("tags: %v", sess.Tags)
	}
	if len([]rune(sess.Title)) > systemSessionTitleMax+1 || !strings.HasPrefix(sess.Title, "Hedef: ") {
		t.Fatalf("title: %q", sess.Title)
	}
	msgs, err := rt.db.ListMessages(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Role != "user" || msgs[0].Text != "PROMPT" || msgs[1].Role != "assistant" || msgs[1].Text != "REPLY" {
		t.Fatalf("messages: %+v", msgs)
	}
	if msgs[0].AuthorKind != db.AuthorUser || msgs[1].AuthorKind != db.AuthorAgent || msgs[1].AgentID != editor.ID {
		t.Fatalf("participants: %+v", msgs)
	}

	// No workspace row for the role: the caller's agent owns the session.
	other, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Alice"})
	id2 := rt.recordSystemAgentSession(ctx, "recipe-optimizer", other, "", "P", "R")
	sess2, _ := rt.db.GetSession(ctx, id2)
	if sess2.AgentID != other.ID || sess2.Title != "recipe-optimizer" {
		t.Fatalf("fallback owner/title: %+v", sess2)
	}
}

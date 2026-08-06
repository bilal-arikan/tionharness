package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSessionMetaAndMessageFields verifies the enriched session.jsonl fields:
// the session header (schema version, labels, status, pinned) and per-message
// enrichment (model, stop reason, usage, duration, feedback) survive a
// close→reopen round-trip and that the setters persist + clear correctly.
func TestSessionMetaAndMessageFields(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ag, _ := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})

	// New sessions are stamped with the current schema version.
	if sess.SchemaVersion != SessionSchemaVersion {
		t.Fatalf("schema version = %d, want %d", sess.SchemaVersion, SessionSchemaVersion)
	}

	// Header setters.
	if err := d.SetSessionPinned(ctx, sess.ID, true); err != nil {
		t.Fatalf("pin: %v", err)
	}

	// An assistant message carrying the enriched per-turn fields.
	msg, err := d.AddMessage(ctx, Message{
		SessionID:  sess.ID,
		Role:       "assistant",
		AgentID:    ag.ID,
		Text:       "hi",
		Model:      "claude-sonnet-4-6",
		StopReason: "end_turn",
		DurationMs: 4200,
		Usage:      &MessageUsage{InputTokens: 100, OutputTokens: 20, CacheReadTokens: 50},
	})
	if err != nil {
		t.Fatalf("add message: %v", err)
	}
	if err := d.SetMessageFeedback(ctx, sess.ID, msg.ID, 1, "good"); err != nil {
		t.Fatalf("feedback: %v", err)
	}
	_ = d.Close()

	// Reopen and verify everything reloaded from disk.
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.SchemaVersion != SessionSchemaVersion {
		t.Errorf("schema version not persisted: %d", got.SchemaVersion)
	}
	if !got.Pinned {
		t.Errorf("pinned not persisted")
	}

	msgs, _ := d2.ListMessages(ctx, sess.ID)
	if len(msgs) != 1 {
		t.Fatalf("message count = %d", len(msgs))
	}
	m := msgs[0]
	if m.Model != "claude-sonnet-4-6" || m.StopReason != "end_turn" || m.DurationMs != 4200 {
		t.Errorf("per-turn fields lost: model=%q stop=%q dur=%d", m.Model, m.StopReason, m.DurationMs)
	}
	if m.Usage == nil || m.Usage.InputTokens != 100 || m.Usage.OutputTokens != 20 || m.Usage.CacheReadTokens != 50 {
		t.Errorf("usage lost: %+v", m.Usage)
	}
	if m.Feedback == nil || m.Feedback.Rating != 1 || m.Feedback.Note != "good" {
		t.Errorf("feedback lost: %+v", m.Feedback)
	}

	// Clearing feedback (rating 0, empty note) removes it.
	if err := d2.SetMessageFeedback(ctx, sess.ID, msg.ID, 0, ""); err != nil {
		t.Fatalf("clear feedback: %v", err)
	}
	msgs, _ = d2.ListMessages(ctx, sess.ID)
	if msgs[0].Feedback != nil {
		t.Errorf("feedback not cleared: %+v", msgs[0].Feedback)
	}
}

// TestSessionModelSnapshot verifies the session header's Model field is seeded from
// the agent at creation and can be updated via SetSessionModel (P1.1).
func TestSessionModelSnapshot(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Agent with a known model.
	ag, err := d.CreateAgent(ctx, Agent{Name: "A", Provider: "anthropic", Model: "claude-sonnet-4-20250514"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Session picks up the agent's model at creation.
	sess, err := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("session model = %q, want claude-sonnet-4-20250514", sess.Model)
	}

	// Round-trip: model survives close→reopen.
	_ = d.Close()
	d2, err := Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := d2.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("model lost on disk: %q", got.Model)
	}

	// SetSessionModel updates the header.
	if err := d2.SetSessionModel(ctx, sess.ID, "claude-opus-4-20250805"); err != nil {
		t.Fatalf("set session model: %v", err)
	}
	got, _ = d2.GetSession(ctx, sess.ID)
	if got.Model != "claude-opus-4-20250805" {
		t.Fatalf("model not updated: %q", got.Model)
	}

	// Session without AgentID leaves Model empty.
	ag2, _ := d2.CreateAgent(ctx, Agent{Name: "B", Provider: "minimax", Model: "minimax-m2.5"})
	sess2, err := d2.CreateSession(ctx, Session{Title: "orphan"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if sess2.Model != "" {
		t.Errorf("session without AgentID must have empty model, got %q", sess2.Model)
	}
	_ = ag2 // used
}

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
	if err := d.SetSessionLabels(ctx, sess.ID, []string{"bug", "urgent"}); err != nil {
		t.Fatalf("labels: %v", err)
	}
	if err := d.SetSessionStatus(ctx, sess.ID, "in_progress"); err != nil {
		t.Fatalf("status: %v", err)
	}
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
	if len(got.Labels) != 2 || got.Labels[0] != "bug" {
		t.Errorf("labels = %v", got.Labels)
	}
	if got.Status != "in_progress" {
		t.Errorf("status = %q", got.Status)
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

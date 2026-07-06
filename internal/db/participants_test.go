package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestMessageParticipantNormalization checks that AddMessage back-fills the
// participant fields from Role + the legacy AgentID dual meaning, so both new and
// legacy-shaped writes carry a canonical author/recipient.
func TestMessageParticipantNormalization(t *testing.T) {
	ctx := context.Background()
	d, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ag, _ := d.CreateAgent(ctx, Agent{Name: "Ada"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ag.ID, Title: "T"})

	// A user turn: AgentID carries the legacy "routed recipient" meaning.
	um, _ := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", AgentID: ag.ID, Text: "hi"})
	if um.AuthorKind != AuthorUser || um.AuthorID != UserParticipantID || um.RecipientID != ag.ID {
		t.Errorf("user turn normalization: kind=%q author=%q recipient=%q", um.AuthorKind, um.AuthorID, um.RecipientID)
	}
	// An assistant turn: author is the agent.
	am, _ := d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", AgentID: ag.ID, Text: "hey"})
	if am.AuthorKind != AuthorAgent || am.AuthorID != ag.ID {
		t.Errorf("assistant turn normalization: kind=%q author=%q", am.AuthorKind, am.AuthorID)
	}
	// Explicit fields are respected (not overwritten): agent addressing a peer.
	pm, _ := d.AddMessage(ctx, Message{
		SessionID: sess.ID, Role: "assistant", AgentID: ag.ID, Text: "@bo",
		AuthorKind: AuthorAgent, AuthorID: ag.ID, RecipientID: "AGT-bo",
	})
	if pm.RecipientID != "AGT-bo" {
		t.Errorf("explicit recipient overwritten: %q", pm.RecipientID)
	}
}

// TestSessionParticipantRosterSelfHeals verifies that agents authoring/addressed
// via the append hot-path join the roster, and that the roster is rebuilt from
// the message lines on reload (the append path never rewrites the header).
func TestSessionParticipantRosterSelfHeals(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "store")
	d, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	ada, _ := d.CreateAgent(ctx, Agent{Name: "Ada"})
	kai, _ := d.CreateAgent(ctx, Agent{Name: "Kai"})
	sess, _ := d.CreateSession(ctx, Session{AgentID: ada.ID, Title: "T"})

	// Ada answers, then the user routes a turn to Kai and Kai answers.
	d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", AgentID: ada.ID, Text: "hi"})
	d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", AgentID: ada.ID, Text: "ben Ada"})
	d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "user", AgentID: kai.ID, Text: "sen?"})
	d.AddMessage(ctx, Message{SessionID: sess.ID, Role: "assistant", AgentID: kai.ID, Text: "ben Kai"})

	got, _ := d.GetSession(ctx, sess.ID)
	if !hasAll(got.Participants, ada.ID, kai.ID) {
		t.Fatalf("in-memory roster missing an agent: %v", got.Participants)
	}
	if containsStr(got.Participants, UserParticipantID) {
		t.Errorf("user must not be stored in the roster: %v", got.Participants)
	}
	_ = d.Close()

	// Reload: the header kept a stale (single-agent) roster from creation, so the
	// loader must rebuild it from the message lines.
	d2, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got2, _ := d2.GetSession(ctx, sess.ID)
	if !hasAll(got2.Participants, ada.ID, kai.ID) {
		t.Errorf("roster not self-healed on reload: %v", got2.Participants)
	}
	// SessionParticipants falls back to the default agent for a legacy empty roster.
	if p := SessionParticipants(Session{AgentID: "AGT-x"}); len(p) != 1 || p[0] != "AGT-x" {
		t.Errorf("SessionParticipants fallback: %v", p)
	}
}

func containsStr(list []string, want string) bool {
	for _, x := range list {
		if x == want {
			return true
		}
	}
	return false
}

func hasAll(list []string, want ...string) bool {
	for _, w := range want {
		if !containsStr(list, w) {
			return false
		}
	}
	return true
}

package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestLabelMultiAgentHistory verifies that, in a session shared by two agents,
// each assistant turn is prefixed with its author ("(you)" for the responder),
// the session is reported multi-author, and a single-agent session is untouched.
func TestLabelMultiAgentHistory(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()

	ada, _ := database.CreateAgent(ctx, db.Agent{Name: "Ada"})
	kai, _ := database.CreateAgent(ctx, db.Agent{Name: "Kai"})

	history := []db.Message{
		{Role: "user", AgentID: ada.ID, Text: "selam"},
		{Role: "assistant", AgentID: ada.ID, Text: "ben Ada"},
		{Role: "user", AgentID: kai.ID, Text: "sen nasilsin"},
		{Role: "assistant", AgentID: kai.ID, Text: "ben Kai"},
	}

	s := &Server{}

	// Responder is Kai: Ada's turn is tagged "[Ada]", Kai's own is "[Kai (you)]",
	// and user turns are tagged with the agent they were directed at.
	out, multi := s.labelMultiAgentHistory(ctx, database, kai.ID, history)
	if !multi {
		t.Fatal("expected multiAgent=true for a two-author session")
	}
	if out[1].Text != "[Ada]: ben Ada" {
		t.Errorf("other-agent turn mislabelled: %q", out[1].Text)
	}
	if out[3].Text != "[Kai (you)]: ben Kai" {
		t.Errorf("own turn mislabelled: %q", out[3].Text)
	}
	if out[0].Text != "[User → Ada]: selam" {
		t.Errorf("directed user turn mislabelled: %q", out[0].Text)
	}
	if out[2].Text != "[User → Kai (you)]: sen nasilsin" {
		t.Errorf("directed user turn (to responder) mislabelled: %q", out[2].Text)
	}
	// The input slice must not be mutated (labels applied on a copy).
	if history[1].Text != "ben Ada" {
		t.Errorf("input history was mutated: %q", history[1].Text)
	}

	// Single-author history answered by the SAME agent → pure self, returned
	// unchanged, multiAgent=false (natural transcript + caching preserved).
	solo := []db.Message{
		{Role: "user", Text: "selam"},
		{Role: "assistant", AgentID: ada.ID, Text: "ben Ada"},
	}
	out2, multi2 := s.labelMultiAgentHistory(ctx, database, ada.ID, solo)
	if multi2 {
		t.Error("single-author session must report multiAgent=false")
	}
	if out2[1].Text != "ben Ada" {
		t.Errorf("single-agent turn must be unlabelled: %q", out2[1].Text)
	}

	// Handover: history authored only by Ada, now answered by Kai. Kai must see
	// Ada's turn attributed so it doesn't mistake it for its own — even though
	// only ONE agent has spoken so far.
	out3, multi3 := s.labelMultiAgentHistory(ctx, database, kai.ID, solo)
	if !multi3 {
		t.Fatal("handover (prior author != responder) must report multiAgent=true")
	}
	if out3[1].Text != "[Ada]: ben Ada" {
		t.Errorf("handover prior-author turn mislabelled: %q", out3[1].Text)
	}
}

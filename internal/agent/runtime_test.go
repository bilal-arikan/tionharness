package agent

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// TestSystemPromptInjectsWorkspaceInstructions verifies that workspace
// instructions are appended to an agent's persona, and omitted when unset.
func TestSystemPromptInjectsWorkspaceInstructions(t *testing.T) {
	agent := db.Agent{Soul: "You are Ada.", Identity: "A helpful assistant."}
	r := &Runtime{}

	// No instructions set → identical to the bare persona.
	if got, want := r.systemPrompt(agent), buildSystemPrompt(agent); got != want {
		t.Fatalf("without instructions: got %q, want %q", got, want)
	}

	// With instructions → persona plus a labelled workspace block.
	r.SetInstructions("  Always answer in Turkish.  ")
	got := r.systemPrompt(agent)
	if !strings.Contains(got, "You are Ada.") {
		t.Errorf("persona dropped: %q", got)
	}
	if !strings.Contains(got, "# Workspace Instructions\nAlways answer in Turkish.") {
		t.Errorf("instructions not injected (or not trimmed): %q", got)
	}

	// Empty/whitespace instructions → no workspace block.
	r.SetInstructions("   ")
	if strings.Contains(r.systemPrompt(agent), "Workspace Instructions") {
		t.Errorf("blank instructions should not add a block: %q", r.systemPrompt(agent))
	}
}

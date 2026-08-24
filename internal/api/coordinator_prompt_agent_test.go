package api

import (
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// buildLeadForTest mirrors the gate in buildStaticPrefix: the coordinator lead
// (manual + the agent's own coordinator prompt) is composed ONLY when the session
// is actually coordinating. Non-coordinator sessions get "" — no manual, and no
// agent prompt either.
func buildLeadForTest(session db.Session, agentRow db.Agent, manual string) string {
	if !session.IsCoordinator() {
		return ""
	}
	return coordinatorLeadBlock(manual, agentRow.CoordinatorPrompt, "", "")
}

// TestCoordinatorPromptInjectedOnlyWhileCoordinating is the core contract of
// Agent.CoordinatorPrompt: present in the assembled context while coordinator
// mode is on, entirely absent when it is off.
func TestCoordinatorPromptInjectedOnlyWhileCoordinating(t *testing.T) {
	const manual = "COORDINATOR MANUAL: drive your workers."
	const prompt = "Spawn one worker per subsystem and review before merging."
	agentRow := db.Agent{Name: "PM", CoordinatorPrompt: prompt}

	// Coordinator mode ON → the prompt rides in the context, behind the manual.
	on := buildLeadForTest(db.Session{ID: "SES1", CoordinatorMode: true}, agentRow, manual)
	if !strings.Contains(on, prompt) {
		t.Fatalf("coordinator prompt missing while coordinating:\n%s", on)
	}
	if !strings.Contains(on, manual) {
		t.Fatalf("shared manual must still be injected alongside the agent prompt:\n%s", on)
	}
	// Order matters: the agent's own direction comes AFTER the shared manual.
	if strings.Index(on, manual) > strings.Index(on, prompt) {
		t.Fatalf("agent prompt must follow the manual, got:\n%s", on)
	}

	// Coordinator mode OFF → nothing at all, not even the manual.
	off := buildLeadForTest(db.Session{ID: "SES2"}, agentRow, manual)
	if strings.Contains(off, prompt) {
		t.Fatalf("coordinator prompt leaked into a non-coordinator session:\n%s", off)
	}
	if off != "" {
		t.Fatalf("non-coordinator session must get an empty lead, got:\n%s", off)
	}
}

// TestCoordinatorLeadBlockDropsEmptyParts pins the zero-cost promise: an agent
// with no coordinator prompt yields exactly the manual — no trailing separator,
// no empty header, nothing extra to pay tokens for.
func TestCoordinatorLeadBlockDropsEmptyParts(t *testing.T) {
	const manual = "MANUAL"
	if got := coordinatorLeadBlock(manual, "", "", ""); got != manual {
		t.Fatalf("empty coordinator prompt must add nothing, got %q", got)
	}
	// Whitespace-only is treated as empty too.
	if got := coordinatorLeadBlock(manual, "   \n\t ", "", ""); got != manual {
		t.Fatalf("whitespace-only coordinator prompt must add nothing, got %q", got)
	}
	// All parts present keep their documented order.
	got := coordinatorLeadBlock("M", "A", "R", "S")
	if got != "M\n\nA\n\nR\n\nS" {
		t.Fatalf("unexpected lead composition: %q", got)
	}
}

// TestCoordinatorPromptRoundTrips verifies the field survives create + partial
// update through the agent store, and that an agent saved without it loads with
// the empty zero value (no migration needed).
func TestCoordinatorPromptRoundTrips(t *testing.T) {
	ctx := t.Context()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	created, err := database.CreateAgent(ctx, db.Agent{Name: "PM", CoordinatorPrompt: "delegate widely"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if created.CoordinatorPrompt != "delegate widely" {
		t.Fatalf("create dropped the prompt: %q", created.CoordinatorPrompt)
	}
	got, err := database.GetAgent(ctx, created.ID)
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if got.CoordinatorPrompt != "delegate widely" {
		t.Fatalf("prompt did not persist: %q", got.CoordinatorPrompt)
	}

	// A patch that does not mention the field leaves it alone.
	name := "PM2"
	updated, err := database.UpdateAgent(ctx, created.ID, db.AgentProfilePatch{Name: &name})
	if err != nil {
		t.Fatalf("update agent: %v", err)
	}
	if updated.CoordinatorPrompt != "delegate widely" {
		t.Fatalf("unrelated patch clobbered the prompt: %q", updated.CoordinatorPrompt)
	}

	// An explicit empty string clears it.
	empty := ""
	cleared, err := database.UpdateAgent(ctx, created.ID, db.AgentProfilePatch{CoordinatorPrompt: &empty})
	if err != nil {
		t.Fatalf("clear prompt: %v", err)
	}
	if cleared.CoordinatorPrompt != "" {
		t.Fatalf("explicit empty patch must clear the prompt, got %q", cleared.CoordinatorPrompt)
	}

	// An agent created without the field loads with the zero value.
	plain, err := database.CreateAgent(ctx, db.Agent{Name: "Solo"})
	if err != nil {
		t.Fatalf("create plain agent: %v", err)
	}
	if plain.CoordinatorPrompt != "" {
		t.Fatalf("absent field must default to empty, got %q", plain.CoordinatorPrompt)
	}
}

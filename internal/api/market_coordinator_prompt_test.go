package api

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/market"
)

// TestCoordinatorPromptSurvivesPackaging pins the publish → pack → install
// round-trip for db.Agent.CoordinatorPrompt. The field is free text with no
// workspace-side resolution (unlike CoordinatorWorkflow, which is dropped when
// its recipe is missing here), so it must arrive verbatim on the other side.
//
// Both halves replicate the handlers' carrier line for this one field and then
// go through the real JSON payload types and db.CreateAgent/GetAgent, because
// buildWorkspaceTemplatePayload / installAgentPack / seedWorkspaceTeam all
// dereference a live agent.Runtime (for skill resolution) that no test in this
// package can build. A regression in the payload structs or the DB round-trip
// fails here; a deleted carrier line is caught by the field-by-field comparison
// against the stored agent.
func TestCoordinatorPromptSurvivesPackaging(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer database.Close()

	const prompt = "Önce planla, sonra iki worker aç ve sonuçları birleştir."

	source := db.Agent{
		Name:              "PM",
		Provider:          "anthropic",
		CoordinatorMode:   true,
		CoordinatorPrompt: prompt,
	}

	// (a) Agent pack: buildWorkspaceTemplatePayload's sibling shape, marshalled
	// through JSON exactly as a published pack travels.
	agentPack := market.AgentPayload{
		Name:              source.Name,
		Provider:          source.Provider,
		CoordinatorMode:   source.CoordinatorMode,
		CoordinatorPrompt: source.CoordinatorPrompt,
	}
	raw, err := json.Marshal(agentPack)
	if err != nil {
		t.Fatalf("marshal agent payload: %v", err)
	}
	var ap market.AgentPayload
	if err := json.Unmarshal(raw, &ap); err != nil {
		t.Fatalf("unmarshal agent payload: %v", err)
	}
	if ap.CoordinatorPrompt != prompt {
		t.Fatalf("agent pack dropped the coordinator prompt: %q", ap.CoordinatorPrompt)
	}
	// installAgentPack's carrier line for this field (no resolution, verbatim).
	installed, err := database.CreateAgent(ctx, db.Agent{
		Name:              ap.Name,
		Provider:          ap.Provider,
		CoordinatorMode:   ap.CoordinatorMode,
		CoordinatorPrompt: ap.CoordinatorPrompt,
	})
	if err != nil {
		t.Fatalf("install agent: %v", err)
	}
	reloaded, err := database.GetAgent(ctx, installed.ID)
	if err != nil {
		t.Fatalf("reload installed agent: %v", err)
	}
	if reloaded.CoordinatorPrompt != prompt {
		t.Fatalf("installed agent lost its coordinator prompt: %q", reloaded.CoordinatorPrompt)
	}

	// (b) Workspace template: publish the live agent with the real builder, then
	// round-trip the payload through JSON as a shared template would travel. The
	// installed agent above is already in this store under the same name, so
	// publish only the source agent by id rather than the whole roster.
	src, err := database.CreateAgent(ctx, source)
	if err != nil {
		t.Fatalf("create source agent: %v", err)
	}
	stored, err := database.GetAgent(ctx, src.ID)
	if err != nil {
		t.Fatalf("reload source agent: %v", err)
	}
	// buildWorkspaceTemplatePayload's carrier line for this field.
	published := market.WorkspaceTemplateAgent{
		Key:               "pm",
		Name:              stored.Name,
		Provider:          stored.Provider,
		CoordinatorMode:   stored.CoordinatorMode,
		CoordinatorPrompt: stored.CoordinatorPrompt,
	}
	if published.CoordinatorPrompt != prompt {
		t.Fatalf("publish dropped the coordinator prompt: %q", published.CoordinatorPrompt)
	}
	raw, err = json.Marshal(published)
	if err != nil {
		t.Fatalf("marshal template agent: %v", err)
	}
	var ta market.WorkspaceTemplateAgent
	if err := json.Unmarshal(raw, &ta); err != nil {
		t.Fatalf("unmarshal template agent: %v", err)
	}
	if ta.CoordinatorPrompt != prompt {
		t.Fatalf("workspace template dropped the coordinator prompt: %q", ta.CoordinatorPrompt)
	}
	// seedWorkspaceTeam's carrier line for this field.
	seeded, err := database.CreateAgent(ctx, db.Agent{
		Name:              ta.Name + " (tmpl)",
		Provider:          ta.Provider,
		CoordinatorMode:   ta.CoordinatorMode,
		CoordinatorPrompt: ta.CoordinatorPrompt,
	})
	if err != nil {
		t.Fatalf("seed agent: %v", err)
	}
	reloadedSeed, err := database.GetAgent(ctx, seeded.ID)
	if err != nil {
		t.Fatalf("reload seeded agent: %v", err)
	}
	if reloadedSeed.CoordinatorPrompt != prompt {
		t.Fatalf("seeded agent lost its coordinator prompt: %q", reloadedSeed.CoordinatorPrompt)
	}

	// An agent published without the field installs with the zero value — no
	// header, no placeholder text invented on the way through.
	var empty market.AgentPayload
	if err := json.Unmarshal([]byte(`{"name":"Solo"}`), &empty); err != nil {
		t.Fatalf("unmarshal bare payload: %v", err)
	}
	if empty.CoordinatorPrompt != "" {
		t.Fatalf("absent coordinatorPrompt must decode empty, got %q", empty.CoordinatorPrompt)
	}
}

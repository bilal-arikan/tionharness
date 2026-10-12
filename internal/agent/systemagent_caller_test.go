package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Ownerless internal jobs run and are billed as their role's system agent, never
// as the default or newest ordinary agent.
func TestSystemRoleCallerIsTheSystemAgent(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Newest User Agent", Provider: "codex-cli"}); err != nil {
		t.Fatal(err)
	}
	insight, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Insight Analyzer", System: true, SystemKey: "insight", Provider: "claude-cli", Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	titler, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Titler", System: true, SystemKey: "titler", Provider: "claude-cli", Model: "haiku"})
	if err != nil {
		t.Fatal(err)
	}
	user, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Mine"})

	got, err := rt.pickInsightAgent(ctx, "")
	if err != nil || got.ID != insight.ID {
		t.Fatalf("pickInsightAgent(\"\") = %s, %v; want insight system agent", got.ID, err)
	}
	// A legacy ordinary analysis agent id is ignored.
	if got, _ := rt.pickInsightAgent(ctx, user.ID); got.ID != insight.ID {
		t.Fatalf("pickInsightAgent(user) = %s, want insight system agent", got.ID)
	}
	if got, err := rt.resolveTitlerAgent(ctx, ""); err != nil || got.ID != titler.ID {
		t.Fatalf("resolveTitlerAgent(\"\") = %s, %v; want titler system agent", got.ID, err)
	}
	// The owner of the titled work still wins.
	if got, _ := rt.resolveTitlerAgent(ctx, user.ID); got.ID != user.ID {
		t.Fatalf("resolveTitlerAgent(owner) = %s, want owner", got.ID)
	}
}

// run_subagent profiles follow the caller's transport while the profile's system
// agent names none (the built-in), and switch to a customisation's pinned
// provider/model once the user set them.
func TestEphemeralSubagentTransport(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedSystemAgents(t, rt)
	caller := db.Agent{ID: "AGTC", Name: "caller", Provider: "codex-cli", ProviderInstanceID: "codex-cli", Model: "gpt-5"}
	p, ok := rt.subagentProfile("explore")
	if !ok {
		t.Fatal("explore profile missing")
	}

	eph := rt.ephemeralSubagent(caller, p)
	if eph.Provider != "codex-cli" || eph.Model != "gpt-5" || eph.ID != caller.ID {
		t.Fatalf("built-in profile = %s/%s (id %s), want caller transport", eph.Provider, eph.Model, eph.ID)
	}

	builtin, _ := rt.db.FindBuiltinAgentBySystemKey("subagent-explore")
	custom, err := rt.db.DeriveAgent(ctx, builtin.ID, db.DeriveAgentOptions{BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	provider, model := "claude-cli", "sonnet"
	if _, err := rt.db.UpdateAgent(ctx, custom.ID, db.AgentProfilePatch{Provider: &provider, ProviderInstanceID: &provider, Model: &model}); err != nil {
		t.Fatal(err)
	}
	eph = rt.ephemeralSubagent(caller, p)
	if eph.Provider != "claude-cli" || eph.Model != "sonnet" || eph.ID != caller.ID {
		t.Fatalf("customised profile = %s/%s (id %s), want claude-cli/sonnet billed to caller", eph.Provider, eph.Model, eph.ID)
	}
}

package db

import (
	"context"
	"errors"
	"testing"
)

func TestEnsureSystemAgentsIdempotent(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "titler", Name: "Titler", SystemPrompt: "title prompt", SuggestedModel: "haiku", AllowedTools: "[]"},
		{SystemKey: "insight", Name: "Insight", SystemPrompt: "insight prompt", SuggestedModel: "haiku", AllowedTools: "[]", Disabled: true},
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	titler, ok := d.FindAgentBySystemKey("titler")
	if !ok {
		t.Fatal("titler not seeded")
	}
	insight, ok := d.FindAgentBySystemKey("insight")
	if !ok || !insight.Disabled {
		t.Fatalf("insight seeded state = (%v, %v), want present and disabled", ok, insight != nil && insight.Disabled)
	}
	customPrompt := "user-customized title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &customPrompt}); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != len(defs) {
		t.Fatalf("agent count = %d, want %d", len(agents), len(defs))
	}
	got, _ := d.FindAgentBySystemKey("titler")
	if got.Soul != customPrompt {
		t.Fatalf("custom prompt clobbered: got %q", got.Soul)
	}
}

func TestEnsureSystemAgentsBackfillsMissingDefinition(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "titler", Name: "Titler", SystemPrompt: "title prompt"},
		{SystemKey: "compactor", Name: "Compactor", SystemPrompt: "summary prompt"},
	}
	if err := d.EnsureSystemAgents(ctx, defs[0]); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.FindAgentBySystemKey("compactor"); ok {
		t.Fatal("compactor unexpectedly present before backfill")
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.FindAgentBySystemKey("compactor"); !ok {
		t.Fatal("missing compactor was not backfilled")
	}
}

func TestFindAgentBySystemKey(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := d.CreateAgent(ctx, Agent{Name: "Titler", System: true, SystemKey: "titler"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		key  string
		want bool
	}{
		{name: "hit", key: "titler", want: true},
		{name: "miss", key: "missing", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := d.FindAgentBySystemKey(tc.key)
			if ok != tc.want {
				t.Fatalf("ok = %v, want %v", ok, tc.want)
			}
			if ok && got.ID != created.ID {
				t.Fatalf("ID = %q, want %q", got.ID, created.ID)
			}
		})
	}
}

func TestDeleteSystemAgentGuard(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	agent, err := d.CreateAgent(ctx, Agent{Name: "Titler", System: true, SystemKey: "titler"})
	if err != nil {
		t.Fatal(err)
	}
	err = d.DeleteAgent(ctx, agent.ID)
	if !errors.Is(err, ErrSystemAgentDelete) {
		t.Fatalf("DeleteAgent error = %v, want actionable system-agent guard", err)
	}
	if got, ok := d.FindAgentBySystemKey("titler"); !ok || got.ID != agent.ID {
		t.Fatal("guarded agent no longer available")
	}
}

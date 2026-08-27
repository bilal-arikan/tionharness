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

// TestEnsureSystemAgentsAdoptsLegacyWorkerAgent covers the migration away from
// materialized "worker:<profile>" agents: the existing row is adopted in place
// (so its sessions keep resolving) and becomes the system agent, with no second
// copy created.
func TestEnsureSystemAgentsAdoptsLegacyWorkerAgent(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := d.CreateAgent(ctx, Agent{Name: "worker:coder", Provider: "anthropic", AllowedTools: `["Read"]`})
	if err != nil {
		t.Fatal(err)
	}
	def := SystemAgentDefinition{
		SystemKey: "subagent-coder", Name: "Worker: Coder", Description: "Implements focused code changes.",
		SystemPrompt: "coder prompt", AllowedTools: `["Read","Write"]`,
	}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	got, ok := d.FindAgentBySystemKey("subagent-coder")
	if !ok {
		t.Fatal("subagent-coder not present after migration")
	}
	if got.ID != legacy.ID {
		t.Fatalf("legacy agent not adopted: got %q, want %q", got.ID, legacy.ID)
	}
	if got.Name != def.Name || got.AllowedTools != def.AllowedTools || !got.System {
		t.Fatalf("adopted agent = %+v, want the system definition applied", got)
	}
	// Idempotent: a second pass changes nothing and adds no duplicate.
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	agents, _ := d.ListAgents(ctx)
	if len(agents) != 1 {
		t.Fatalf("agent count = %d, want 1", len(agents))
	}
}

// TestEnsureSystemAgentsDisablesRedundantLegacyWorker covers the other migration
// branch: when the system agent already exists, the leftover "worker:<profile>"
// row is disabled instead of competing with it as a second target.
func TestEnsureSystemAgentsDisablesRedundantLegacyWorker(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def := SystemAgentDefinition{
		SystemKey: "subagent-coder", Name: "Worker: Coder",
		SystemPrompt: "coder prompt", AllowedTools: `["Read","Write"]`,
	}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	legacy, err := d.CreateAgent(ctx, Agent{Name: "worker:coder", Provider: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	got, err := d.GetAgent(ctx, legacy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Disabled {
		t.Fatal("redundant legacy worker agent left enabled")
	}
	if _, ok := d.FindAgentBySystemKey("subagent-coder"); !ok {
		t.Fatal("system agent lost")
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
		{SystemKey: "compaction", Name: "Compactor", SystemPrompt: "compact prompt"},
	}
	if err := d.EnsureSystemAgents(ctx, defs[0]); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.FindAgentBySystemKey("compaction"); ok {
		t.Fatal("compaction unexpectedly present before backfill")
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.FindAgentBySystemKey("compaction"); !ok {
		t.Fatal("missing compaction agent was not backfilled")
	}
}

func TestEnsureSystemAgentsMigratesCustomizedCompactorInPlace(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := d.CreateAgent(ctx, Agent{
		Name: "Customized Legacy Name", Soul: "custom overview prompt", Model: "custom-model",
		System: true, SystemKey: "compactor", Disabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "overview-summarizer", Name: "Overview Summarizer", SystemPrompt: "default overview", SuggestedModel: "haiku"},
		{SystemKey: "compaction", Name: "Compactor", SystemPrompt: "default compact", SuggestedModel: "haiku"},
	}

	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	migrated, ok := d.FindAgentBySystemKey("overview-summarizer")
	if !ok {
		t.Fatal("overview-summarizer missing after migration")
	}
	if migrated.ID != legacy.ID {
		t.Fatalf("migrated ID = %q, want preserved %q", migrated.ID, legacy.ID)
	}
	if migrated.Name != legacy.Name || migrated.Soul != legacy.Soul || migrated.Model != legacy.Model || migrated.Disabled != legacy.Disabled {
		t.Fatalf("customized fields changed during migration: got %+v, want %+v", *migrated, legacy)
	}
	if _, ok := d.FindAgentBySystemKey("compactor"); ok {
		t.Fatal("legacy compactor key remains after migration")
	}
	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("agent count = %d, want 2 without duplicate overview row", len(agents))
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

// TestEnsureSystemAgentsSeedsVisualIdentity covers the "seed, never re-impose"
// rule for Avatar/Color: a row that predates the visual identity is backfilled
// on the next boot, while a value the user picked survives re-seeding.
func TestEnsureSystemAgentsSeedsVisualIdentity(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// A pre-existing row without any visual identity, as every workspace seeded
	// before this field existed looks on disk.
	legacy, err := d.CreateAgent(ctx, Agent{Name: "Titler", System: true, SystemKey: "titler"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateAgent(ctx, Agent{Name: "Compactor", System: true, SystemKey: "compaction", Avatar: "🙂", Color: "#123456"}); err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "titler", Name: "Titler", AllowedTools: "[]", Avatar: "🔖", Color: "#7C6BE8"},
		{SystemKey: "compaction", Name: "Compactor", AllowedTools: "[]", Avatar: "📦", Color: "#6B5FA8"},
		{SystemKey: "insight", Name: "Insight", AllowedTools: "[]", Avatar: "🔮", Color: "#5C6480"},
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}

	titler, ok := d.FindAgentBySystemKey("titler")
	if !ok {
		t.Fatal("titler missing")
	}
	if titler.ID != legacy.ID {
		t.Fatalf("titler row replaced: got %s, want %s", titler.ID, legacy.ID)
	}
	if titler.Avatar != "🔖" || titler.Color != "#7C6BE8" {
		t.Fatalf("empty visual identity not backfilled: avatar=%q color=%q", titler.Avatar, titler.Color)
	}

	// A user-chosen avatar/color must survive every subsequent boot.
	compaction, ok := d.FindAgentBySystemKey("compaction")
	if !ok {
		t.Fatal("compaction missing")
	}
	if compaction.Avatar != "🙂" || compaction.Color != "#123456" {
		t.Fatalf("user visual identity clobbered: avatar=%q color=%q", compaction.Avatar, compaction.Color)
	}

	// A freshly created system agent carries the canonical identity immediately.
	insight, ok := d.FindAgentBySystemKey("insight")
	if !ok {
		t.Fatal("insight missing")
	}
	if insight.Avatar != "🔮" || insight.Color != "#5C6480" {
		t.Fatalf("new system agent visual identity = (%q, %q)", insight.Avatar, insight.Color)
	}
}

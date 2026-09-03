package db

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// TestEnsureSystemAgentsIdempotent covers the built-in contract: every
// definition seeds exactly one LOCKED row, a second pass adds nothing, and the
// locked row refuses edits (customisation goes through DeriveAgent).
func TestEnsureSystemAgentsIdempotent(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "titler", Name: "Titler", SystemPrompt: "title prompt", SuggestedModel: "haiku", AllowedTools: "[]"},
		{SystemKey: "insight", Name: "Insight", SystemPrompt: "insight prompt", SuggestedModel: "haiku", AllowedTools: "[]"},
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	titler, ok := d.FindAgentBySystemKey("titler")
	if !ok || !titler.Locked {
		t.Fatalf("titler seeded = (%v, locked=%v), want a locked built-in", ok, titler != nil && titler.Locked)
	}
	// A built-in is never disabled: it IS the fallback.
	insight, ok := d.FindAgentBySystemKey("insight")
	if !ok || insight.Disabled {
		t.Fatalf("insight seeded state = (%v, disabled=%v), want present and enabled", ok, insight != nil && insight.Disabled)
	}
	customPrompt := "user-customized title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &customPrompt}); !errors.Is(err, ErrAgentLocked) {
		t.Fatalf("UpdateAgent on locked built-in = %v, want ErrAgentLocked", err)
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
	if got.Soul != "title prompt" {
		t.Fatalf("built-in prompt drifted: got %q", got.Soul)
	}
}

// TestEnsureSystemAgentsReimposesCanonical covers the "owned by code" rule: a
// hand-edited built-in row is brought back to the definition on the next boot.
func TestEnsureSystemAgentsReimposesCanonical(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def := SystemAgentDefinition{SystemKey: "titler", Name: "Titler", SystemPrompt: "title prompt", SuggestedModel: "haiku", AllowedTools: "[]", Avatar: "🔖", Color: "#7C6BE8"}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	titler, _ := d.FindAgentBySystemKey("titler")
	if _, err := d.mutateAgentLocked(titler.ID, func(a *Agent) {
		a.Soul = "tampered"
		a.Model = "opus"
		a.Disabled = true
	}); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	got, _ := d.FindAgentBySystemKey("titler")
	if got.ID != titler.ID || got.Soul != "title prompt" || got.Model != "haiku" || got.Disabled || got.Avatar != "🔖" {
		t.Fatalf("canonical values not re-imposed: %+v", *got)
	}
}

// TestEnsureSystemAgentsAdoptsLegacyWorkerAgent covers the migration away from
// materialized "worker:<profile>" agents: the existing row is adopted in place
// (so its sessions keep resolving) and becomes the LOCKED built-in — whatever
// provider/allowlist it carried is replaced by the definition.
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
		t.Fatalf("legacy agent not adopted in place: got %q, want %q", got.ID, legacy.ID)
	}
	if got.Name != def.Name || got.AllowedTools != def.AllowedTools || !got.System || !got.Locked || got.Provider != "claude-cli" || got.Soul != "coder prompt" {
		t.Fatalf("adopted agent = %+v, want the locked definition", got)
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
// row is disabled and parked under the built-in instead of competing with it.
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
	if serving, ok := d.FindAgentBySystemKey("subagent-coder"); !ok || !serving.Locked {
		t.Fatal("built-in should keep serving the role")
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

// TestEnsureSystemAgentsMigratesCustomizedCompactorInPlace covers a customised
// legacy row: its key is renamed and it becomes the LOCKED built-in with the
// same id; the legacy edits (name, soul, model, disabled) are dropped because
// customisation now lives in a derived child.
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
	if migrated.ID != legacy.ID || !migrated.Locked || migrated.Disabled {
		t.Fatalf("legacy row should be the locked built-in in place: %+v", *migrated)
	}
	if migrated.Name != "Overview Summarizer" || migrated.Soul != "default overview" || migrated.Model != "haiku" {
		t.Fatalf("legacy edits should be replaced by the definition: %+v", *migrated)
	}
	if _, ok := d.FindAgentBySystemKey("compactor"); ok {
		t.Fatal("legacy compactor key remains after migration")
	}
	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 2 {
		t.Fatalf("agent count = %d, want 2 (one built-in per definition)", len(agents))
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

// TestFindAgentBySystemKeyPrefersEnabledCustomisation pins the role-resolution
// order: enabled customisation > locked built-in > disabled customisation.
func TestFindAgentBySystemKeyPrefersEnabledCustomisation(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def := SystemAgentDefinition{SystemKey: "titler", Name: "Titler", SystemPrompt: "title prompt", SuggestedModel: "haiku"}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	builtin, _ := d.FindBuiltinAgentBySystemKey("titler")
	child, err := d.DeriveAgent(ctx, builtin.ID, DeriveAgentOptions{Name: "My Titler", BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := d.FindAgentBySystemKey("titler"); got.ID != child.ID {
		t.Fatalf("enabled customisation should serve the role; got %q", got.ID)
	}
	disabled := true
	if _, err := d.UpdateAgent(ctx, child.ID, AgentProfilePatch{Disabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.FindAgentBySystemKey("titler"); got.ID != builtin.ID {
		t.Fatalf("built-in should serve the role while the customisation is disabled; got %q", got.ID)
	}
	// A second enabled customisation of the same role is refused.
	if _, err := d.DeriveAgent(ctx, builtin.ID, DeriveAgentOptions{BindRole: true}); err != nil {
		t.Fatalf("deriving while the other customisation is disabled should succeed: %v", err)
	}
	enabled := false
	if _, err := d.UpdateAgent(ctx, child.ID, AgentProfilePatch{Disabled: &enabled}); !errors.Is(err, ErrSystemRoleTaken) {
		t.Fatalf("re-enabling next to another enabled customisation = %v, want ErrSystemRoleTaken", err)
	}
}

func TestDeleteSystemAgentGuard(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := d.CreateAgent(ctx, Agent{Name: "Titler", System: true, SystemKey: "titler", Locked: true})
	if err != nil {
		t.Fatal(err)
	}
	err = d.DeleteAgent(ctx, builtin.ID)
	if !errors.Is(err, ErrSystemAgentDelete) {
		t.Fatalf("DeleteAgent error = %v, want actionable built-in guard", err)
	}
	if got, ok := d.FindAgentBySystemKey("titler"); !ok || got.ID != builtin.ID {
		t.Fatal("guarded agent no longer available")
	}
	// A workspace customisation of the role IS deletable: the role falls back
	// to the built-in.
	child, err := d.DeriveAgent(ctx, builtin.ID, DeriveAgentOptions{BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteAgent(ctx, child.ID); err != nil {
		t.Fatalf("deleting a customisation = %v, want nil", err)
	}
	if got, _ := d.FindAgentBySystemKey("titler"); got.ID != builtin.ID {
		t.Fatalf("role did not fall back to the built-in after deleting its customisation: %q", got.ID)
	}
}

// TestEnsureSystemAgentsImposesVisualIdentity: a built-in always carries the
// canonical avatar/colour — a legacy row's own pick is replaced (the place for
// a custom look is a derived child).
func TestEnsureSystemAgentsImposesVisualIdentity(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := d.CreateAgent(ctx, Agent{Name: "Titler", System: true, SystemKey: "titler"})
	if err != nil {
		t.Fatal(err)
	}
	custom, err := d.CreateAgent(ctx, Agent{Name: "Compactor", System: true, SystemKey: "compaction", Avatar: "🙂", Color: "#123456"})
	if err != nil {
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
	titler, _ := d.FindAgentBySystemKey("titler")
	if titler.ID != legacy.ID || !titler.Locked || titler.Avatar != "🔖" || titler.Color != "#7C6BE8" {
		t.Fatalf("titler row should be the built-in in place with the canonical look: %+v", *titler)
	}
	compaction, _ := d.FindAgentBySystemKey("compaction")
	if compaction.ID != custom.ID || !compaction.Locked || compaction.Avatar != "📦" || compaction.Color != "#6B5FA8" {
		t.Fatalf("legacy custom look should be replaced on the built-in: %+v", *compaction)
	}
	insight, _ := d.FindAgentBySystemKey("insight")
	if insight.Avatar != "🔮" || insight.Color != "#5C6480" {
		t.Fatalf("new system agent visual identity = (%q, %q)", insight.Avatar, insight.Color)
	}
	if agents, _ := d.ListAgents(ctx); len(agents) != 3 {
		t.Fatalf("agent count = %d, want 3", len(agents))
	}
}

// TestEnsureSystemAgentsImposesProvider: the built-in's provider is the
// definition's, whatever the legacy row stored (empty or user-picked).
func TestEnsureSystemAgentsImposesProvider(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "titler", Name: "Titler", SystemPrompt: "title prompt", SuggestedModel: "haiku", AllowedTools: "[]", Provider: "claude-cli"},
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	titler, _ := d.FindAgentBySystemKey("titler")
	if provider, instanceID := readAgentProviderFieldsFromDisk(t, d, titler.ID); provider != "claude-cli" || instanceID != "claude-cli" {
		t.Fatalf("seeded provider fields on disk = (%q, %q), want (%q, %q)", provider, instanceID, "claude-cli", "claude-cli")
	}
	legacy, err := d.CreateAgent(ctx, Agent{Name: "Legacy Insight", System: true, SystemKey: "insight"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) {
		a.Provider = ""
		a.ProviderInstanceID = ""
	}); err != nil {
		t.Fatal(err)
	}
	custom, err := d.CreateAgent(ctx, Agent{Name: "Custom Compactor", System: true, SystemKey: "compaction", Provider: "anthropic"})
	if err != nil {
		t.Fatal(err)
	}
	defs = append(defs,
		SystemAgentDefinition{SystemKey: "insight", Name: "Insight", SystemPrompt: "insight prompt", AllowedTools: "[]"},
		SystemAgentDefinition{SystemKey: "compaction", Name: "Compactor", SystemPrompt: "compact prompt", AllowedTools: "[]", Provider: "claude-cli"},
	)
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{legacy.ID, custom.ID} {
		if provider, instanceID := readAgentProviderFieldsFromDisk(t, d, id); provider != "claude-cli" || instanceID != "claude-cli" {
			t.Fatalf("%s provider fields on disk = (%q, %q), want (%q, %q)", id, provider, instanceID, "claude-cli", "claude-cli")
		}
	}
}

// migratedArtifactFixture builds the state the FIRST cut of the migration left
// real workspaces in: the legacy row (with sessions) parked as a child under a
// newer, unreferenced locked built-in.
func migratedArtifactFixture(t *testing.T, d *DB, key string, canonical Agent) (legacy, builtin Agent) {
	t.Helper()
	ctx := context.Background()
	legacy, err := d.CreateAgent(ctx, Agent{Name: canonical.Name, Soul: "old prompt", Provider: "codex-cli", Model: "gpt-5.6-sol", System: true, SystemKey: key})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateSession(ctx, Session{AgentID: legacy.ID, Title: "worker run"}); err != nil {
		t.Fatal(err)
	}
	builtin, err = d.CreateAgent(ctx, canonical)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.mutateAgentLocked(builtin.ID, func(a *Agent) { a.CreatedAt = legacy.CreatedAt + 10 }); err != nil {
		t.Fatal(err)
	}
	if _, err := d.mutateAgentLocked(legacy.ID, func(a *Agent) {
		a.ParentID = builtin.ID
		a.Overrides = []string{"soul", "provider", "model"}
	}); err != nil {
		t.Fatal(err)
	}
	return legacy, builtin
}

// TestEnsureSystemAgentsCollapsesMigrationArtifact covers the repair for
// workspaces migrated by the first cut of the inheritance model: the old row
// (the one worker sessions reference) becomes the built-in in place and the
// migration-created locked row disappears. A customisation derived AFTER the
// built-in existed is never touched.
func TestEnsureSystemAgentsCollapsesMigrationArtifact(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def := SystemAgentDefinition{SystemKey: "subagent-coder", Name: "Worker: Coder", SystemPrompt: "coder prompt v2", AllowedTools: `["Read","Write"]`, Provider: "claude-cli"}
	legacy, builtin := migratedArtifactFixture(t, d, "subagent-coder", def.canonicalAgent())

	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	got, ok := d.FindBuiltinAgentBySystemKey("subagent-coder")
	if !ok || got.ID != legacy.ID || !got.Locked || got.ParentID != "" || len(got.Overrides) != 0 {
		t.Fatalf("legacy row not collapsed into the built-in: %+v", got)
	}
	if got.Soul != "coder prompt v2" || got.Provider != "claude-cli" || got.Model != "" {
		t.Fatalf("canonical values not imposed on the collapsed row: %+v", got)
	}
	if _, err := d.GetAgent(ctx, builtin.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("migration-created built-in should be gone, got err=%v", err)
	}
	if agents, _ := d.ListAgents(ctx); len(agents) != 1 {
		t.Fatalf("agent count = %d, want 1", len(agents))
	}

	// A deliberate customisation (derived after the built-in) survives boots.
	child, err := d.DeriveAgent(ctx, legacy.ID, DeriveAgentOptions{BindRole: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.mutateAgentLocked(child.ID, func(a *Agent) { a.CreatedAt = legacy.CreatedAt + 20 }); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	if kept, err := d.GetAgent(ctx, child.ID); err != nil || kept.Locked || kept.ParentID != legacy.ID {
		t.Fatalf("deliberate customisation was touched: err=%v agent=%+v", err, kept)
	}
	if serving, _ := d.FindAgentBySystemKey("subagent-coder"); serving.ID != child.ID {
		t.Fatalf("customisation should serve the role: %q", serving.ID)
	}
}

// TestEnsureSystemAgentsKeepsArtifactWhenBuiltinHasSessions: a locked row a
// session already points at is not a fresh artifact, so nothing is collapsed.
func TestEnsureSystemAgentsKeepsArtifactWhenBuiltinHasSessions(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	def := SystemAgentDefinition{SystemKey: "titler", Name: "Titler", SystemPrompt: "p2"}
	legacy, builtin := migratedArtifactFixture(t, d, "titler", def.canonicalAgent())
	if _, err := d.CreateSession(ctx, Session{AgentID: builtin.ID, Title: "t"}); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, def); err != nil {
		t.Fatal(err)
	}
	if got, _ := d.FindBuiltinAgentBySystemKey("titler"); got.ID != builtin.ID {
		t.Fatalf("built-in with sessions was replaced: %q", got.ID)
	}
	if kept, err := d.GetAgent(ctx, legacy.ID); err != nil || kept.ParentID != builtin.ID {
		t.Fatalf("child was collapsed despite the built-in being referenced: err=%v %+v", err, kept)
	}
}

func readAgentProviderFieldsFromDisk(t *testing.T, d *DB, agentID string) (string, string) {
	t.Helper()
	path, err := d.AgentPath(agentID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var row struct {
		Provider           string `json:"provider"`
		ProviderInstanceID string `json:"providerInstanceId"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatal(err)
	}
	return row.Provider, row.ProviderInstanceID
}

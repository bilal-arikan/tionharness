package db

import (
	"context"
	"errors"
	"testing"
)

// systemTestDefs is the small registry the edit tests seed from.
func systemTestDefs() []SystemAgentDefinition {
	return []SystemAgentDefinition{
		{SystemKey: "titler", Name: "Titler", Description: "names chats", SystemPrompt: "title prompt", SuggestedModel: "haiku", AllowedTools: "[]"},
		{SystemKey: "insight", Name: "Insight", SystemPrompt: "insight prompt", SuggestedModel: "haiku", AllowedTools: "[]"},
	}
}

// newSeededDB opens a store with the app-global customisation layer attached and
// the built-ins seeded, returning the store and the shared layer.
func newSeededDB(t *testing.T, dataDir string) (*DB, *GlobalSystemAgentOverrides) {
	t.Helper()
	ctx := context.Background()
	layer, err := OpenGlobalSystemAgentOverrides(dataDir)
	if err != nil {
		t.Fatalf("open override layer: %v", err)
	}
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	d.SetGlobalSystemAgentOverrides(layer)
	if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return d, layer
}

func builtinByKey(t *testing.T, d *DB, key string) Agent {
	t.Helper()
	a, ok := d.FindBuiltinAgentBySystemKey(key)
	if !ok {
		t.Fatalf("no built-in for %q", key)
	}
	return *a
}

// The point of the whole change: a built-in is edited IN PLACE, with no derived
// copy, and the edit sticks.
func TestUpdateBuiltinSystemAgentEditsInPlace(t *testing.T) {
	ctx := context.Background()
	d, _ := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")

	custom := "my own title prompt"
	updated, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom})
	if err != nil {
		t.Fatalf("UpdateAgent on built-in: %v", err)
	}
	if updated.Soul != custom {
		t.Errorf("soul = %q, want %q", updated.Soul, custom)
	}
	if updated.ID != titler.ID {
		t.Errorf("id = %q, want the same row %q - an edit must not create a copy", updated.ID, titler.ID)
	}
	agents, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, a := range agents {
		if a.SystemKey == "titler" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("rows serving titler = %d, want 1", count)
	}
	serving, ok := d.FindAgentBySystemKey("titler")
	if !ok || serving.ID != titler.ID || serving.Soul != custom {
		t.Errorf("role resolves to %+v, want the edited built-in", serving)
	}
}

// The edit must survive the re-impose that runs on every boot - that is what the
// app-global layer exists for.
func TestBuiltinEditSurvivesReseeding(t *testing.T) {
	ctx := context.Background()
	d, _ := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")

	custom := "my own title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatalf("re-seed: %v", err)
	}
	after := builtinByKey(t, d, "titler")
	if after.Soul != custom {
		t.Errorf("soul after re-seed = %q, want the customised %q", after.Soul, custom)
	}
}

// A field the user never touched keeps following the compiled registry, so an
// app upgrade still reaches it.
func TestUntouchedFieldsStillTrackTheRegistry(t *testing.T) {
	ctx := context.Background()
	d, _ := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")

	custom := "my own title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); err != nil {
		t.Fatal(err)
	}
	next := systemTestDefs()
	next[0].SuggestedModel = "sonnet"
	next[0].Description = "names chats, better"
	if err := d.EnsureSystemAgents(ctx, next...); err != nil {
		t.Fatal(err)
	}
	after := builtinByKey(t, d, "titler")
	if after.Soul != custom {
		t.Errorf("soul = %q, want the customisation to survive", after.Soul)
	}
	if after.Model != "sonnet" {
		t.Errorf("model = %q, want the new compiled value - an untouched field must follow the registry", after.Model)
	}
	if after.Identity != "names chats, better" {
		t.Errorf("identity = %q, want the new compiled description", after.Identity)
	}
}

// The customisation is stored app-globally, so a SECOND workspace store seeded
// from the same layer starts out already carrying it.
func TestBuiltinEditAppliesToEveryWorkspace(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	first, layer := newSeededDB(t, dataDir)
	titler := builtinByKey(t, first, "titler")

	custom := "my own title prompt"
	if _, err := first.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); err != nil {
		t.Fatal(err)
	}

	second, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second.SetGlobalSystemAgentOverrides(layer)
	if err := second.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatal(err)
	}
	other := builtinByKey(t, second, "titler")
	if other.Soul != custom {
		t.Errorf("second workspace soul = %q, want the shared customisation %q", other.Soul, custom)
	}
}

// Restoring drops the customisation everywhere, so the role follows the compiled
// definition again.
func TestClearBuiltinOverridesRestoresTheCompiledDefinition(t *testing.T) {
	ctx := context.Background()
	d, layer := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")

	custom := "my own title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); err != nil {
		t.Fatal(err)
	}
	restored, err := d.ClearBuiltinSystemAgentOverrides(ctx, titler.ID)
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if restored.Soul != "title prompt" {
		t.Errorf("soul = %q, want the compiled prompt back", restored.Soul)
	}
	if _, ok := layer.Get("titler"); ok {
		t.Error("customisation still stored after a restore")
	}
}

// A field edited back to its compiled value stops being a customisation, so it
// resumes following the registry instead of being pinned to a stale copy.
func TestEditingAFieldBackToItsDefaultUnpinsIt(t *testing.T) {
	ctx := context.Background()
	d, layer := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")

	custom := "my own title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); err != nil {
		t.Fatal(err)
	}
	back := "title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &back}); err != nil {
		t.Fatal(err)
	}
	if ov, ok := layer.Get("titler"); ok && len(ov.Fields) > 0 {
		t.Errorf("stored fields = %v, want none once the value equals the default", ov.Fields)
	}
}

// A built-in must never be disabled or re-parented: it is the role's fallback.
func TestBuiltinRejectsDisableAndReparent(t *testing.T) {
	ctx := context.Background()
	d, _ := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")

	disabled := true
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Disabled: &disabled}); err == nil {
		t.Error("disabling a built-in succeeded, want an error")
	}
	parent := "AGT1"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{ParentID: &parent}); err == nil {
		t.Error("re-parenting a built-in succeeded, want an error")
	}
}

// With no layer attached there is nowhere durable to write, so the built-in must
// stay read-only rather than accept an edit the next boot would revert.
func TestBuiltinStaysLockedWithoutTheGlobalLayer(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatal(err)
	}
	titler := builtinByKey(t, d, "titler")
	custom := "nope"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); !errors.Is(err, ErrAgentLocked) {
		t.Fatalf("UpdateAgent = %v, want ErrAgentLocked with no layer attached", err)
	}
}

// The customisation is durable: a fresh layer reading the same directory sees it.
func TestCustomisationPersistsAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	d, _ := newSeededDB(t, dataDir)
	titler := builtinByKey(t, d, "titler")

	custom := "my own title prompt"
	if _, err := d.UpdateAgent(ctx, titler.ID, AgentProfilePatch{Soul: &custom}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenGlobalSystemAgentOverrides(dataDir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	ov, ok := reopened.Get("titler")
	if !ok {
		t.Fatal("no customisation after reopen")
	}
	if ov.Values.Soul != custom {
		t.Errorf("stored soul = %q, want %q", ov.Values.Soul, custom)
	}
}

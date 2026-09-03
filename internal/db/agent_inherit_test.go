package db

import (
	"context"
	"errors"
	"testing"
)

func openInheritDB(t *testing.T) (*DB, context.Context) {
	t.Helper()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d, context.Background()
}

func strp(s string) *string { return &s }

// TestDeriveAgentInheritsEverything: a fresh child resolves to its parent's
// values with no overrides, and follows later parent edits.
func TestDeriveAgentInheritsEverything(t *testing.T) {
	d, ctx := openInheritDB(t)
	parent, err := d.CreateAgent(ctx, Agent{Name: "Base", Soul: "base soul", Model: "sonnet", Provider: "anthropic", Color: "#111111", Skills: []string{"a"}, MCPEnabled: true})
	if err != nil {
		t.Fatal(err)
	}
	child, err := d.DeriveAgent(ctx, parent.ID, DeriveAgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if child.Name != "Base (kopya)" || child.ParentID != parent.ID || len(child.Overrides) != 0 {
		t.Fatalf("derived = %+v", child)
	}
	if child.Soul != "base soul" || child.Model != "sonnet" || child.Provider != "anthropic" || child.Color != "#111111" || !child.MCPEnabled || len(child.Skills) != 1 {
		t.Fatalf("child did not inherit: %+v", child)
	}
	if _, err := d.UpdateAgent(ctx, parent.ID, AgentProfilePatch{Soul: strp("new soul"), Model: strp("opus")}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetAgent(ctx, child.ID)
	if got.Soul != "new soul" || got.Model != "opus" {
		t.Fatalf("child did not follow parent edit: soul=%q model=%q", got.Soul, got.Model)
	}
}

// TestUpdateAgentMarksAndResetsOverrides: editing a field on a child pins it;
// resetting it makes the field inherit again.
func TestUpdateAgentMarksAndResetsOverrides(t *testing.T) {
	d, ctx := openInheritDB(t)
	parent, _ := d.CreateAgent(ctx, Agent{Name: "Base", Soul: "base soul", Model: "sonnet"})
	child, _ := d.DeriveAgent(ctx, parent.ID, DeriveAgentOptions{Name: "Kid"})

	got, err := d.UpdateAgent(ctx, child.ID, AgentProfilePatch{Model: strp("haiku"), Name: strp("Kid2")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "haiku" || got.Soul != "base soul" || len(got.Overrides) != 1 || got.Overrides[0] != "model" {
		t.Fatalf("after model edit: model=%q soul=%q overrides=%v", got.Model, got.Soul, got.Overrides)
	}
	// Parent model change does not reach the pinned child field.
	if _, err := d.UpdateAgent(ctx, parent.ID, AgentProfilePatch{Model: strp("opus")}); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetAgent(ctx, child.ID)
	if got.Model != "haiku" {
		t.Fatalf("override lost: model=%q", got.Model)
	}
	got, err = d.UpdateAgent(ctx, child.ID, AgentProfilePatch{ResetFields: []string{"model"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model != "opus" || len(got.Overrides) != 0 {
		t.Fatalf("after reset: model=%q overrides=%v", got.Model, got.Overrides)
	}
	if _, err := d.UpdateAgent(ctx, child.ID, AgentProfilePatch{ResetFields: []string{"nope"}}); err == nil {
		t.Fatal("unknown reset key accepted")
	}
}

// TestUpdateAgentToolsPinsToolUnit: the instant-save tools endpoint pins the
// tool trio on a child; ClearAgentOverrides releases every override.
func TestUpdateAgentToolsPinsToolUnit(t *testing.T) {
	d, ctx := openInheritDB(t)
	parent, _ := d.CreateAgent(ctx, Agent{Name: "Base", MCPEnabled: true})
	child, _ := d.DeriveAgent(ctx, parent.ID, DeriveAgentOptions{})
	if err := d.UpdateAgentTools(ctx, child.ID, false, `{"Bash":"blocked"}`); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetAgent(ctx, child.ID)
	if got.MCPEnabled || got.BlockedTools != `["Bash"]` || !overrideSet(got.Overrides)["tools"] {
		t.Fatalf("tools not pinned: %+v", got)
	}
	cleared, err := d.ClearAgentOverrides(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !cleared.MCPEnabled || cleared.BlockedTools != "[]" || len(cleared.Overrides) != 0 {
		t.Fatalf("overrides not cleared: %+v", cleared)
	}
}

// TestReparentTransitions covers root→child (keeps behaviour, owns all),
// child→root (materialises), cycles and unknown parents.
func TestReparentTransitions(t *testing.T) {
	d, ctx := openInheritDB(t)
	a, _ := d.CreateAgent(ctx, Agent{Name: "A", Soul: "A soul", Model: "opus"})
	b, _ := d.CreateAgent(ctx, Agent{Name: "B", Soul: "B soul", Model: "haiku"})

	got, err := d.UpdateAgent(ctx, b.ID, AgentProfilePatch{ParentID: strp(a.ID)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Soul != "B soul" || got.Model != "haiku" || len(got.Overrides) != len(InheritableFieldKeys()) {
		t.Fatalf("root→child must keep behaviour by owning every field: %+v", got)
	}
	got, _ = d.UpdateAgent(ctx, b.ID, AgentProfilePatch{ResetFields: []string{"soul"}})
	if got.Soul != "A soul" {
		t.Fatalf("reset after re-parent should inherit: %q", got.Soul)
	}
	// Cycle: A cannot inherit from its own child.
	if _, err := d.UpdateAgent(ctx, a.ID, AgentProfilePatch{ParentID: strp(b.ID)}); !errors.Is(err, ErrAgentParentCycle) {
		t.Fatalf("cycle accepted: %v", err)
	}
	if _, err := d.UpdateAgent(ctx, a.ID, AgentProfilePatch{ParentID: strp(a.ID)}); !errors.Is(err, ErrAgentParentCycle) {
		t.Fatalf("self-parent accepted: %v", err)
	}
	if _, err := d.UpdateAgent(ctx, b.ID, AgentProfilePatch{ParentID: strp("AGT999")}); !errors.Is(err, ErrAgentParentNotFound) {
		t.Fatalf("unknown parent accepted: %v", err)
	}
	// child → root freezes the effective values.
	got, err = d.UpdateAgent(ctx, b.ID, AgentProfilePatch{ParentID: strp("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.ParentID != "" || len(got.Overrides) != 0 || got.Soul != "A soul" || got.Model != "haiku" {
		t.Fatalf("child→root did not materialise: %+v", got)
	}
	if _, err := d.UpdateAgent(ctx, a.ID, AgentProfilePatch{Soul: strp("changed")}); err != nil {
		t.Fatal(err)
	}
	if got, _ = d.GetAgent(ctx, b.ID); got.Soul != "A soul" {
		t.Fatalf("detached agent still follows former parent: %q", got.Soul)
	}
}

// TestDeleteReparentsChildren: removing a middle layer keeps every descendant's
// effective values, folding the removed layer into overrides.
func TestDeleteReparentsChildren(t *testing.T) {
	d, ctx := openInheritDB(t)
	root, _ := d.CreateAgent(ctx, Agent{Name: "Root", Soul: "root soul", Model: "opus", Color: "#000000"})
	mid, _ := d.DeriveAgent(ctx, root.ID, DeriveAgentOptions{Name: "Mid"})
	if _, err := d.UpdateAgent(ctx, mid.ID, AgentProfilePatch{Model: strp("haiku")}); err != nil {
		t.Fatal(err)
	}
	leaf, _ := d.DeriveAgent(ctx, mid.ID, DeriveAgentOptions{Name: "Leaf"})
	if _, err := d.UpdateAgent(ctx, leaf.ID, AgentProfilePatch{Color: strp("#ffffff")}); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteAgent(ctx, mid.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetAgent(ctx, leaf.ID)
	if got.ParentID != root.ID {
		t.Fatalf("leaf parent = %q, want grandparent %q", got.ParentID, root.ID)
	}
	if got.Model != "haiku" || got.Color != "#ffffff" || got.Soul != "root soul" {
		t.Fatalf("leaf effective values changed: %+v", got)
	}
	set := overrideSet(got.Overrides)
	if !set["model"] || !set["color"] || set["soul"] {
		t.Fatalf("overrides after reparent = %v, want model+color only", got.Overrides)
	}
	// Deleting the root turns the leaf into a root with frozen values.
	if err := d.DeleteAgent(ctx, root.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = d.GetAgent(ctx, leaf.ID)
	if got.ParentID != "" || len(got.Overrides) != 0 || got.Soul != "root soul" || got.Model != "haiku" {
		t.Fatalf("leaf not materialised after root delete: %+v", got)
	}
}

// TestDeriveBindRoleRequiresSystemParent: BindRole only makes sense under a
// system agent; a plain derive from a built-in yields a normal agent.
func TestDeriveBindRoleRequiresSystemParent(t *testing.T) {
	d, ctx := openInheritDB(t)
	plain, _ := d.CreateAgent(ctx, Agent{Name: "Plain"})
	if _, err := d.DeriveAgent(ctx, plain.ID, DeriveAgentOptions{BindRole: true}); !errors.Is(err, ErrAgentParentNotFound) {
		t.Fatalf("bind on non-system parent = %v", err)
	}
	if err := d.EnsureSystemAgents(ctx, SystemAgentDefinition{SystemKey: "titler", Name: "Titler", SystemPrompt: "p"}); err != nil {
		t.Fatal(err)
	}
	builtin, _ := d.FindBuiltinAgentBySystemKey("titler")
	variant, err := d.DeriveAgent(ctx, builtin.ID, DeriveAgentOptions{Name: "Variant"})
	if err != nil {
		t.Fatal(err)
	}
	if variant.System || variant.SystemKey != "" || variant.Locked {
		t.Fatalf("unbound derive must be a plain agent: %+v", variant)
	}
	if got, _ := d.FindAgentBySystemKey("titler"); got.ID != builtin.ID {
		t.Fatalf("unbound variant must not take the role: %q", got.ID)
	}
	if _, err := d.DeriveAgent(ctx, "AGT404", DeriveAgentOptions{}); !errors.Is(err, ErrAgentParentNotFound) {
		t.Fatalf("derive from missing parent = %v", err)
	}
}

// TestCreateAgentValidatesInheritance: CreateAgent rejects a dangling parent
// and unknown override keys, and drops overrides on a root.
func TestCreateAgentValidatesInheritance(t *testing.T) {
	d, ctx := openInheritDB(t)
	if _, err := d.CreateAgent(ctx, Agent{Name: "X", ParentID: "AGT404"}); !errors.Is(err, ErrAgentParentNotFound) {
		t.Fatalf("dangling parent = %v", err)
	}
	if _, err := d.CreateAgent(ctx, Agent{Name: "X", Overrides: []string{"bogus"}}); err == nil {
		t.Fatal("unknown override key accepted")
	}
	root, err := d.CreateAgent(ctx, Agent{Name: "X", Overrides: []string{"soul"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(root.Overrides) != 0 {
		t.Fatalf("root kept overrides: %v", root.Overrides)
	}
}

// TestSessionSeedsFromEffectiveAgent: session creation reads the RESOLVED
// agent (model snapshot, coordinator default), not the child's raw cache.
func TestSessionSeedsFromEffectiveAgent(t *testing.T) {
	d, ctx := openInheritDB(t)
	parent, _ := d.CreateAgent(ctx, Agent{Name: "Base", Model: "opus", CoordinatorMode: true, CoordinatorWorkflow: "wf"})
	child, _ := d.DeriveAgent(ctx, parent.ID, DeriveAgentOptions{})
	if _, err := d.UpdateAgent(ctx, parent.ID, AgentProfilePatch{Model: strp("sonnet")}); err != nil {
		t.Fatal(err)
	}
	s, err := d.CreateSession(ctx, Session{AgentID: child.ID, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "sonnet" || !s.CoordinatorMode || s.CoordinatorWorkflow != "wf" {
		t.Fatalf("session seeded from raw row: model=%q coordinator=%v wf=%q", s.Model, s.CoordinatorMode, s.CoordinatorWorkflow)
	}
}

package db

import (
	"context"
	"testing"
)

// deriveCustomisation reproduces the OLD way of customising a built-in: a child
// bound to the same system role, overriding one field.
func deriveCustomisation(t *testing.T, d *DB, builtinID, soul string) Agent {
	t.Helper()
	ctx := context.Background()
	child, err := d.DeriveAgent(ctx, builtinID, DeriveAgentOptions{Name: "Titler (Atölye)", BindRole: true})
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	updated, err := d.UpdateAgent(ctx, child.ID, AgentProfilePatch{Soul: &soul})
	if err != nil {
		t.Fatalf("customise: %v", err)
	}
	return updated
}

// The migration: a customisation copy made under the old model is folded into
// the app-global layer on the next boot, so the user's edit survives as the
// built-in's own value and the copy stops serving the role.
func TestAdoptsAnExistingCustomisationIntoTheGlobalLayer(t *testing.T) {
	ctx := context.Background()
	d, layer := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")
	custom := "the prompt the user wrote"
	child := deriveCustomisation(t, d, titler.ID, custom)

	// The next boot re-seeds, which is where adoption happens.
	if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatalf("re-seed: %v", err)
	}

	ov, ok := layer.Get("titler")
	if !ok {
		t.Fatal("customisation was not adopted into the app-global layer")
	}
	if ov.Values.Soul != custom {
		t.Errorf("adopted soul = %q, want %q", ov.Values.Soul, custom)
	}
	// The built-in now carries the user's value directly.
	after := builtinByKey(t, d, "titler")
	if after.Soul != custom {
		t.Errorf("built-in soul = %q, want the adopted %q", after.Soul, custom)
	}
	// And the role resolves to the built-in, not to the copy.
	serving, ok := d.FindAgentBySystemKey("titler")
	if !ok || serving.ID != titler.ID {
		t.Errorf("role serves %v, want the built-in %q", serving, titler.ID)
	}
	_ = child
}

// The copy is retired, not destroyed: the sessions it ran must keep resolving
// their author's name and avatar.
func TestAdoptionRetiresTheCopyWithoutDestroyingIt(t *testing.T) {
	ctx := context.Background()
	d, _ := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")
	child := deriveCustomisation(t, d, titler.ID, "the prompt the user wrote")

	if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatal(err)
	}

	// Still readable by id (history rendering reads through GetAgent).
	kept, err := d.GetAgent(ctx, child.ID)
	if err != nil {
		t.Fatalf("retired copy is gone: %v", err)
	}
	if kept.Name != child.Name {
		t.Errorf("name = %q, want the copy to keep its identity %q", kept.Name, child.Name)
	}
	// But out of the roster and no longer bound to the role.
	if !kept.Disabled {
		t.Error("retired copy is still enabled")
	}
	if kept.SystemKey != "" {
		t.Errorf("systemKey = %q, want the copy unbound from the role", kept.SystemKey)
	}
}

// Adoption runs once: a second boot finds nothing left to adopt and must not
// disturb the customisation already in force.
func TestAdoptionIsIdempotent(t *testing.T) {
	ctx := context.Background()
	d, layer := newSeededDB(t, t.TempDir())
	titler := builtinByKey(t, d, "titler")
	custom := "the prompt the user wrote"
	deriveCustomisation(t, d, titler.ID, custom)

	for i := 0; i < 3; i++ {
		if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
			t.Fatalf("re-seed %d: %v", i, err)
		}
	}
	ov, ok := layer.Get("titler")
	if !ok || ov.Values.Soul != custom {
		t.Errorf("customisation after repeated boots = %+v, want the adopted prompt", ov)
	}
	if after := builtinByKey(t, d, "titler"); after.Soul != custom {
		t.Errorf("built-in soul = %q, want %q", after.Soul, custom)
	}
}

// A role that was never customised must not gain an override entry just because
// a boot ran.
func TestAdoptionStoresNothingForAnUncustomisedRole(t *testing.T) {
	ctx := context.Background()
	d, layer := newSeededDB(t, t.TempDir())

	if err := d.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
		t.Fatal(err)
	}
	if keys := layer.Keys(); len(keys) != 0 {
		t.Errorf("stored customisations = %v, want none", keys)
	}
}

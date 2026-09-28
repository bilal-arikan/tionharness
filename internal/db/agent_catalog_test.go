package db

import (
	"path/filepath"
	"testing"
)

func openCatalogTestDB(t *testing.T, path string) *DB {
	t.Helper()
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestAgentCatalogMigrationSharingAndRestart(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	one := openCatalogTestDB(t, filepath.Join(root, "one"))
	two := openCatalogTestDB(t, filepath.Join(root, "two"))
	a, err := one.CreateAgent(ctx, Agent{Name: "Same name", Soul: "first", Model: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := two.CreateAgent(ctx, Agent{Name: "Same name", Soul: "second"})
	if err != nil {
		t.Fatal(err)
	}
	catalog := openCatalogTestDB(t, filepath.Join(root, "catalog"))
	catalog.MarkAgentCatalog()
	if err := one.AttachAgentCatalog(catalog, "WS1"); err != nil {
		t.Fatal(err)
	}
	if err := two.AttachAgentCatalog(catalog, "WS2"); err != nil {
		t.Fatal(err)
	}
	a, _ = one.GetAgent(ctx, a.ID)
	b, _ = two.GetAgent(ctx, b.ID)
	if a.CatalogID == b.CatalogID || a.CatalogID == "" {
		t.Fatal("independent profiles were merged")
	}
	assigned, err := two.AssignCatalogAgent(ctx, a.CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	name := "Shared name"
	generation := one.MutationGen()
	if _, err := two.UpdateAgent(ctx, assigned.ID, AgentProfilePatch{Name: &name}); err != nil {
		t.Fatal(err)
	}
	if err := two.UpdateAgentTools(ctx, assigned.ID, false, `{"shell":"blocked"}`); err != nil {
		t.Fatal(err)
	}
	first, _ := one.GetAgent(ctx, a.ID)
	if one.MutationGen() <= generation {
		t.Fatal("shared edit did not invalidate workspace views")
	}
	central, _ := catalog.GetAgent(ctx, a.CatalogID)
	if first.Name != name || central.Name != name || first.MCPEnabled || first.ToolOverrides != `{"shell":"blocked"}` {
		t.Fatalf("shared edit not visible: %+v", first)
	}
	unchanged, _ := two.GetAgent(ctx, b.ID)
	if unchanged.Soul != "second" {
		t.Fatal("unrelated profile changed")
	}
	if err := two.DetachCatalogAgent(ctx, a.CatalogID); err != nil {
		t.Fatal(err)
	}
	history, _ := two.GetAgent(ctx, assigned.ID)
	if history.RunnableErr() == nil {
		t.Fatal("detached assignment remains runnable")
	}
	if !history.Deleted || !history.CatalogDetached || history.Name != name {
		t.Fatal("history not preserved")
	}
	restored, err := two.AssignCatalogAgent(ctx, a.CatalogID)
	if err != nil || restored.ID != assigned.ID || restored.Deleted {
		t.Fatalf("reassignment: %+v %v", restored, err)
	}
	// Reopen from disk: old workspace snapshots must never overwrite central edits.
	_ = one.Close()
	_ = two.Close()
	_ = catalog.Close()
	catalog = openCatalogTestDB(t, filepath.Join(root, "catalog"))
	one = openCatalogTestDB(t, filepath.Join(root, "one"))
	if err := one.AttachAgentCatalog(catalog, "WS1"); err != nil {
		t.Fatal(err)
	}
	after, _ := one.GetAgent(ctx, a.ID)
	if after.Name != name || after.ID != a.ID {
		t.Fatalf("restart lost settings/identity: %+v", after)
	}
	rows, _ := catalog.ListAgents(ctx)
	if len(rows) != 2 {
		t.Fatalf("migration is not idempotent: %d rows", len(rows))
	}
}

func TestAgentCatalogInheritanceAndFreshAgents(t *testing.T) {
	ctx := t.Context()
	catalog := openCatalogTestDB(t, filepath.Join(t.TempDir(), "catalog"))
	one := openCatalogTestDB(t, filepath.Join(t.TempDir(), "one"))
	two := openCatalogTestDB(t, filepath.Join(t.TempDir(), "two"))
	if err := one.AttachAgentCatalog(catalog, "WS1"); err != nil {
		t.Fatal(err)
	}
	if err := two.AttachAgentCatalog(catalog, "WS2"); err != nil {
		t.Fatal(err)
	}
	parent, err := one.CreateAgent(ctx, Agent{Name: "Parent", Soul: "base"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := one.DeriveAgent(ctx, parent.ID, DeriveAgentOptions{Name: "Child"})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := two.AssignCatalogAgent(ctx, child.CatalogID)
	if err != nil {
		t.Fatal(err)
	}
	soul := "edited parent"
	if _, err := one.UpdateAgent(ctx, parent.ID, AgentProfilePatch{Soul: &soul}); err != nil {
		t.Fatal(err)
	}
	got, _ := two.GetAgent(ctx, linked.ID)
	if got.Soul != soul {
		t.Fatal("cross-workspace inheritance lost")
	}
	soul = "child override"
	if _, err := two.UpdateAgent(ctx, linked.ID, AgentProfilePatch{Soul: &soul}); err != nil {
		t.Fatal(err)
	}
	if _, err := two.ClearAgentOverrides(ctx, linked.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = two.GetAgent(ctx, linked.ID)
	if got.Soul != "edited parent" {
		t.Fatal("central reset did not restore inheritance")
	}
	clone, err := one.CreateAgent(ctx, parent)
	if err != nil {
		t.Fatal(err)
	}
	if clone.CatalogID == parent.CatalogID {
		t.Fatal("clone reused catalog identity")
	}
	if err := catalog.DeleteAgent(ctx, child.CatalogID); err != nil {
		t.Fatal(err)
	}
	rows, _ := two.ListAgents(ctx)
	if len(rows) != 0 {
		t.Fatal("deleted central profile remains in workspace roster")
	}
	history, err := two.GetAgent(ctx, linked.ID)
	if err != nil || !history.Deleted || history.RunnableErr() == nil {
		t.Fatal("deleted central profile remains runnable")
	}
}

func TestAgentCatalogBuiltinsShareOneDefinition(t *testing.T) {
	ctx := t.Context()
	catalog, layer := newSeededDB(t, t.TempDir())
	catalog.MarkAgentCatalog()
	one := openCatalogTestDB(t, t.TempDir())
	two := openCatalogTestDB(t, t.TempDir())
	for index, local := range []*DB{one, two} {
		local.SetGlobalSystemAgentOverrides(layer)
		if err := local.AttachAgentCatalog(catalog, []string{"WS1", "WS2"}[index]); err != nil {
			t.Fatal(err)
		}
		if err := local.EnsureSystemAgents(ctx, systemTestDefs()...); err != nil {
			t.Fatal(err)
		}
	}
	a := builtinByKey(t, one, "titler")
	b := builtinByKey(t, two, "titler")
	if a.CatalogID != b.CatalogID {
		t.Fatal("duplicate builtins")
	}
	soul := "shared title prompt"
	if _, err := one.UpdateAgent(ctx, a.ID, AgentProfilePatch{Soul: &soul}); err != nil {
		t.Fatal(err)
	}
	if got := builtinByKey(t, two, "titler"); got.Soul != soul {
		t.Fatal("builtin edit was not shared")
	}
	if err := two.DetachCatalogAgent(ctx, b.CatalogID); err == nil {
		t.Fatal("builtin was detached")
	}
	if _, err := two.ClearBuiltinSystemAgentOverrides(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if got := builtinByKey(t, one, "titler"); got.Soul == soul {
		t.Fatal("builtin reset was not shared")
	}
}

func TestAgentCatalogImportsRepairedLegacySystemIdentity(t *testing.T) {
	ctx := t.Context()
	catalog := openCatalogTestDB(t, t.TempDir())
	catalog.MarkAgentCatalog()
	defs := []SystemAgentDefinition{{SystemKey: "subagent-coder", Name: "Coder", SystemPrompt: "Code carefully"}}
	if err := catalog.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	local := openCatalogTestDB(t, t.TempDir())
	legacy, err := local.CreateAgent(ctx, Agent{Name: "worker:coder"})
	if err != nil {
		t.Fatal(err)
	}
	local.PrepareAgentCatalog("WS1")
	if err := local.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	if err := local.AttachAgentCatalog(catalog, "WS1"); err != nil {
		t.Fatal(err)
	}
	got, err := local.GetAgent(ctx, legacy.ID)
	if err != nil || !got.System || !got.Locked || got.SystemKey != "subagent-coder" {
		t.Fatalf("legacy identity not preserved: %+v %v", got, err)
	}
	rows, _ := catalog.ListAgents(ctx)
	if len(rows) != 1 {
		t.Fatalf("legacy repair created duplicate catalog profiles: %d", len(rows))
	}
}

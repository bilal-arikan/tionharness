package db

import (
	"context"
	"testing"
)

func TestRetireSystemAgentsPreservesHistoryAndActiveRoles(t *testing.T) {
	ctx := context.Background()
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defs := []SystemAgentDefinition{
		{SystemKey: "goal-writer", Name: "Goal Writer"},
		{SystemKey: "workspace-evolver", Name: "Workspace Evolver"},
		{SystemKey: "titler", Name: "Titler"},
	}
	if err := d.EnsureSystemAgents(ctx, defs...); err != nil {
		t.Fatal(err)
	}
	obsolete, _ := d.FindAgentBySystemKey("goal-writer")
	for range 2 {
		if err := d.RetireSystemAgents(ctx, "goal-writer", "workspace-evolver"); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := d.FindAgentBySystemKey("goal-writer"); ok {
		t.Fatal("retired goal writer still resolves")
	}
	if _, ok := d.FindAgentBySystemKey("workspace-evolver"); ok {
		t.Fatal("retired workspace evolver still resolves")
	}
	if _, ok := d.FindAgentBySystemKey("titler"); !ok {
		t.Fatal("active role was retired")
	}
	rows, err := d.ListAgents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].SystemKey != "titler" {
		t.Fatalf("active agents = %+v, want only titler", rows)
	}
	historical, err := d.GetAgent(ctx, obsolete.ID)
	if err != nil || !historical.Deleted {
		t.Fatalf("historical agent = %+v, err = %v, want deleted row", historical, err)
	}
}

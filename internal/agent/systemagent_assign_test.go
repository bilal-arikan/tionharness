package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Assigning one of the role's own system agents selects that row as-is, even
// over an enabled customisation.
func TestResolveSystemAgentAssignedSameRoleSystemAgent(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	if _, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Enabled Compactor", System: true, SystemKey: "compaction", Model: "haiku"}); err != nil {
		t.Fatal(err)
	}
	picked, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Picked Compactor", System: true, SystemKey: "compaction", Model: "sonnet"})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetSystemRoleAssignments(map[string]string{"compaction": picked.ID})
	got, _, err := rt.ResolveSystemAgent("compaction")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != picked.ID || got.Model != "sonnet" {
		t.Fatalf("ResolveSystemAgent = %+v, want assigned row", got)
	}
}

// A regular agent never runs a system role, even if an older settings file
// still names one: resolution ignores it and uses the role's own agent.
func TestResolveSystemAgentIgnoresRegularAgentAssignment(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	role, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Titler", System: true, SystemKey: "titler", Soul: "title prompt"})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Mine", Provider: "codex-cli", Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetSystemRoleAssignments(map[string]string{"titler": mine.ID})
	got, _, err := rt.ResolveSystemAgent("titler")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != role.ID {
		t.Fatalf("ResolveSystemAgent = %s, want the role's own agent %s", got.ID, role.ID)
	}
}

// A stale assignment (agent disabled since) silently falls back to the built-in.
func TestResolveSystemAgentStaleAssignmentFallsBack(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	gone, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Old Titler", System: true, SystemKey: "titler", Disabled: true})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetSystemRoleAssignments(map[string]string{"titler": gone.ID})
	got, usedFallback, err := rt.ResolveSystemAgent("titler")
	if err != nil {
		t.Fatal(err)
	}
	assertSystemAgentFallback(t, got, usedFallback, "titler")
}

func TestCheckSystemRoleAssignment(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	mine, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Mine"})
	other, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Titler", System: true, SystemKey: "titler"})
	same, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "Compactor", System: true, SystemKey: "compaction"})

	cases := []struct {
		key, id string
		want    error
	}{
		{"compaction", same.ID, nil},
		{"compaction", "", nil},
		{"compaction", mine.ID, ErrRoleAssignmentAgent},
		{"compaction", other.ID, ErrRoleAssignmentAgent},
		{"compaction", "missing", ErrRoleAssignmentAgent},
		{"insight-applier", same.ID, ErrRoleAssignmentKey},
		{"nope", "", ErrRoleAssignmentKey},
	}
	for _, c := range cases {
		err := rt.CheckSystemRoleAssignment(ctx, c.key, c.id)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("Check(%s,%s) = %v, want %v", c.key, c.id, err, c.want)
		}
	}
}

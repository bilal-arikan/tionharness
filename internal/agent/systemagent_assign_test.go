package agent

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// A regular agent assigned to a utility role lends only its transport: the role
// keeps its own prompt, id and SystemKey, and the provider is pinned.
func TestResolveSystemAgentAssignedRegularAgentLendsTransport(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	role, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Titler", System: true, SystemKey: "titler", Soul: "title prompt", Model: "haiku", Provider: "claude-cli"})
	if err != nil {
		t.Fatal(err)
	}
	mine, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Mine", Soul: "my soul", Provider: "codex-cli", Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetSystemRoleAssignments(map[string]string{"titler": mine.ID})

	got, _, err := rt.ResolveSystemAgent("titler")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != role.ID || got.Soul != "title prompt" || got.SystemKey != "titler" {
		t.Fatalf("role contract not kept: %+v", got)
	}
	if got.Provider != "codex-cli" || got.Model != "gpt-5" {
		t.Fatalf("transport not borrowed: provider=%q model=%q", got.Provider, got.Model)
	}
	if !pinsProvider(got) || !slices.Contains(got.Overrides, "model") {
		t.Fatalf("assigned transport not pinned: overrides=%v", got.Overrides)
	}
}

// Assigning the role's own system agent selects that row as-is, even over an
// enabled customisation.
func TestResolveSystemAgentAssignedSameRoleSystemAgent(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
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

// A stale assignment (agent archived since) silently falls back to the built-in.
func TestResolveSystemAgentStaleAssignmentFallsBack(t *testing.T) {
	rt := newSystemAgentResolveRuntime(t)
	ctx := context.Background()
	mine, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Gone", Provider: "codex-cli", Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.db.SetAgentArchived(ctx, mine.ID, true); err != nil {
		t.Fatal(err)
	}
	rt.SetSystemRoleAssignments(map[string]string{"titler": mine.ID})
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
		{"compaction", mine.ID, nil},
		{"compaction", same.ID, nil},
		{"compaction", "", nil},
		{"compaction", other.ID, ErrRoleAssignmentAgent},
		{"compaction", "missing", ErrRoleAssignmentAgent},
		{"insight-applier", mine.ID, ErrRoleAssignmentKey},
		{"nope", "", ErrRoleAssignmentKey},
	}
	for _, c := range cases {
		err := rt.CheckSystemRoleAssignment(ctx, c.key, c.id)
		if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
			t.Errorf("Check(%s,%s) = %v, want %v", c.key, c.id, err, c.want)
		}
	}
}

// A worker profile assigned to a regular agent spawns that agent, and its own
// tool list is left alone (the profile allowlist belongs to the built-in row).
func TestWorkerProfileAssignedToRegularAgent(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	seedSystemAgents(t, rt)
	mine, err := rt.db.CreateAgent(ctx, db.Agent{Name: "My Explorer", AllowedTools: `["Read","Write"]`})
	if err != nil {
		t.Fatal(err)
	}
	rt.SetSystemRoleAssignments(map[string]string{"subagent-explore": mine.ID})

	id, err := rt.resolveWorkerTarget(ctx, "", "", "explore")
	if err != nil {
		t.Fatal(err)
	}
	if id != mine.ID {
		t.Fatalf("resolveWorkerTarget = %q, want assigned %q", id, mine.ID)
	}
	if got, _ := rt.db.GetAgent(ctx, mine.ID); got.AllowedTools != `["Read","Write"]` {
		t.Fatalf("assigned agent's tools rewritten: %s", got.AllowedTools)
	}

	sub, ephemeral, err := rt.resolveSubagentTarget(ctx, db.Agent{Name: "caller"}, "explore")
	if err != nil {
		t.Fatal(err)
	}
	if ephemeral || sub.ID != mine.ID {
		t.Fatalf("run_subagent explore = %s (ephemeral=%v), want assigned agent", sub.Name, ephemeral)
	}

	// The profile's prompt still comes from the built-in (ResolveSystemAgent does
	// not reflect a regular worker assignment).
	sa, _, err := rt.ResolveSystemAgent("subagent-explore")
	if err != nil || sa.SystemKey != "subagent-explore" {
		t.Fatalf("ResolveSystemAgent(subagent-explore) = %+v, %v", sa, err)
	}
}

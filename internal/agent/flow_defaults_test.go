package agent

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

func newFlowSeedDB(t *testing.T) (*db.DB, string) {
	t.Helper()
	storeDir := filepath.Join(t.TempDir(), "store")
	database, err := db.Open(storeDir)
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, storeDir
}

// TestEnsureDefaultFlowsSeeds seeds all shipped defaults exactly once and stays
// idempotent across repeated calls (no duplicates on the next open).
func TestEnsureDefaultFlowsSeeds(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newFlowSeedDB(t)

	if err := EnsureDefaultFlows(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	flows, _ := database.ListFlows(ctx)
	if len(flows) != len(defaultFlows) {
		t.Fatalf("want %d seeded flows, got %d", len(defaultFlows), len(flows))
	}
	if flows[0].Seed == "" {
		t.Fatalf("seeded flow missing Seed key")
	}

	// Second call must not duplicate.
	if err := EnsureDefaultFlows(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	flows, _ = database.ListFlows(ctx)
	if len(flows) != len(defaultFlows) {
		t.Fatalf("idempotency broken: got %d flows", len(flows))
	}
}

// TestEnsureDefaultFlowsRespectsDeletion is the core requirement: once the user
// deletes a seeded default, re-running the seeder (next open) must NOT resurrect it.
func TestEnsureDefaultFlowsRespectsDeletion(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newFlowSeedDB(t)

	if err := EnsureDefaultFlows(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	flows, _ := database.ListFlows(ctx)
	if len(flows) == 0 {
		t.Fatalf("no seeded flows")
	}
	if err := database.DeleteFlow(ctx, flows[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// Re-seed (simulates the next workspace open). The deleted default must stay gone.
	if err := EnsureDefaultFlows(ctx, database, storeDir); err != nil {
		t.Fatalf("reseed: %v", err)
	}
	after, _ := database.ListFlows(ctx)
	if len(after) != len(flows)-1 {
		t.Fatalf("deleted default was resurrected: want %d, got %d", len(flows)-1, len(after))
	}
}

// TestEnsureDefaultFlowsAssignsFirstAgent assigns an existing agent to seeded
// agent nodes so the flow is runnable out of the box.
func TestEnsureDefaultFlowsAssignsFirstAgent(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newFlowSeedDB(t)

	ag, err := database.CreateAgent(ctx, db.Agent{Name: "A", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if err := EnsureDefaultFlows(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	flows, _ := database.ListFlows(ctx)
	var g orchestration.Graph
	if err := json.Unmarshal([]byte(flows[0].Graph), &g); err != nil {
		t.Fatalf("unmarshal graph: %v", err)
	}
	for _, n := range g.Nodes {
		if n.Type == orchestration.NodeAgent && n.AgentID != ag.ID {
			t.Fatalf("agent node %q not assigned to first agent: got %q", n.ID, n.AgentID)
		}
	}
}

// TestEnsureDefaultFlowsNoAgentStillSeeds seeds even with zero agents (empty
// agentId), matching an instantiated gallery template the user must fill.
func TestEnsureDefaultFlowsNoAgentStillSeeds(t *testing.T) {
	ctx := context.Background()
	database, storeDir := newFlowSeedDB(t)

	if err := EnsureDefaultFlows(ctx, database, storeDir); err != nil {
		t.Fatalf("seed: %v", err)
	}
	flows, _ := database.ListFlows(ctx)
	if len(flows) != len(defaultFlows) {
		t.Fatalf("want %d seeded flows without agents, got %d", len(defaultFlows), len(flows))
	}
	if !strings.Contains(flows[0].Graph, "\"agentId\"") && !strings.Contains(flows[0].Graph, "answer") {
		t.Fatalf("graph looks malformed: %s", flows[0].Graph)
	}
}

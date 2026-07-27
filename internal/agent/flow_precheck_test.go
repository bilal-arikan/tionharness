package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// newFlowAgent creates an agent backed by the keyless claude-cli provider (the
// empty provider name), which builds without any configuration — so a valid
// graph stays valid in a bare test registry.
func newFlowAgent(t *testing.T, rt *Runtime, name string) db.Agent {
	t.Helper()
	a, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: name})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return a
}

// createFlow persists a graph as a flow and returns its id.
func createFlow(t *testing.T, rt *Runtime, g orchestration.Graph) string {
	t.Helper()
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal graph: %v", err)
	}
	f, err := rt.db.CreateFlow(context.Background(), db.Flow{Name: "test flow", Graph: string(data)})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}
	return f.ID
}

// TestFlowPrecheckValidGraphPasses is the regression guard: a graph whose agents
// exist and whose providers build must pass the precheck untouched.
func TestFlowPrecheckValidGraphPasses(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	a := newFlowAgent(t, rt, "worker")

	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "n1"},
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}", Next: "n2"},
			{ID: "n2", Type: orchestration.NodeTransform, Template: "{{last}}", Next: "n3"},
			{ID: "n3", Type: orchestration.NodeDelay, DelayMs: 10},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("structural validate: %v", err)
	}
	if err := rt.validateFlowPreconditions(context.Background(), g); err != nil {
		t.Fatalf("expected valid graph to pass precheck, got: %v", err)
	}
}

// TestFlowPrecheckMissingAgent rejects a graph referencing an agent that no
// longer exists — the case that previously only failed once the engine reached
// the node.
func TestFlowPrecheckMissingAgent(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	g := orchestration.Graph{
		Start: "n1",
		Nodes: []orchestration.Node{
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: "AGT-gone", Prompt: "hi"},
		},
	}
	err := rt.validateFlowPreconditions(context.Background(), g)
	if err == nil {
		t.Fatal("expected an error for a missing agent, got nil")
	}
	if !strings.Contains(err.Error(), "AGT-gone") || !strings.Contains(err.Error(), "n1") {
		t.Fatalf("error should name the node and the missing agent, got: %v", err)
	}
}

// TestFlowPrecheckUnconfiguredProvider rejects an agent whose provider cannot be
// built (deleted or missing its API key), instead of failing on the node's first
// completion call.
func TestFlowPrecheckUnconfiguredProvider(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	// The bare test registry has no anthropic key, so this provider does not build.
	a, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "keyless", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	g := orchestration.Graph{
		Start: "n1",
		Nodes: []orchestration.Node{
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "hi"},
		},
	}
	perr := rt.validateFlowPreconditions(context.Background(), g)
	if perr == nil {
		t.Fatal("expected an error for an unconfigured provider, got nil")
	}
	if !strings.Contains(perr.Error(), "keyless") {
		t.Fatalf("error should name the agent, got: %v", perr)
	}
}

// TestFlowPrecheckBadNodeParams catches transform/delay nodes whose parameters
// are unusable: an empty template renders to nothing, a negative delay is never
// intentional.
func TestFlowPrecheckBadNodeParams(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	g := orchestration.Graph{
		Start: "t1",
		Nodes: []orchestration.Node{
			{ID: "t1", Type: orchestration.NodeTransform, Template: "   ", Next: "d1"},
			{ID: "d1", Type: orchestration.NodeDelay, DelayMs: -5},
		},
	}
	err := rt.validateFlowPreconditions(context.Background(), g)
	if err == nil {
		t.Fatal("expected errors for an empty template and a negative delay, got nil")
	}
	// Both faults are reported at once, so the user fixes the flow in one pass.
	if !strings.Contains(err.Error(), "t1") || !strings.Contains(err.Error(), "d1") {
		t.Fatalf("expected both offending nodes in the joined error, got: %v", err)
	}
}

// TestFlowPrecheckReportsEveryProblem verifies faults are joined rather than
// short-circuiting on the first one.
func TestFlowPrecheckReportsEveryProblem(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	g := orchestration.Graph{
		Start: "n1",
		Nodes: []orchestration.Node{
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: "AGT-a", Next: "n2"},
			{ID: "n2", Type: orchestration.NodeAgent, AgentID: "AGT-b"},
		},
	}
	err := rt.validateFlowPreconditions(context.Background(), g)
	if err == nil {
		t.Fatal("expected errors, got nil")
	}
	if !strings.Contains(err.Error(), "AGT-a") || !strings.Contains(err.Error(), "AGT-b") {
		t.Fatalf("expected both missing agents reported, got: %v", err)
	}
}

// TestRunFlowRejectsBeforeCreatingRun is the point of the whole change: an
// unrunnable flow must return an error and leave NO FlowRun row behind.
func TestRunFlowRejectsBeforeCreatingRun(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()

	flowID := createFlow(t, rt, orchestration.Graph{
		Start: "n1",
		Nodes: []orchestration.Node{
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: "AGT-gone", Prompt: "hi"},
		},
	})

	if _, err := rt.RunFlow(ctx, flowID, "input", false, nil); err == nil {
		t.Fatal("expected RunFlow to reject a flow referencing a missing agent")
	}

	runs, err := rt.db.ListFlowRuns(ctx, flowID)
	if err != nil {
		t.Fatalf("list flow runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no FlowRun row for a rejected flow, got %d", len(runs))
	}
}

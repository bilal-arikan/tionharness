package agent

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
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
	// The precheck asserts every agent's provider can actually run. The keyless
	// claude-cli provider needs the claude CLI on PATH, so without it a
	// structurally valid graph legitimately fails readiness — skip rather than
	// report a false failure on a runner that has no claude installed.
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("keyless claude-cli provider needs the claude CLI on PATH")
	}

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

// TestValidateFlowGraph_RejectsMissingAgent is the save-time counterpart of
// TestFlowPrecheckMissingAgent: ValidateFlowGraph is what the flow API/tool
// save paths call before persisting a graph, so a flow referencing a
// never-existed or deleted agent must be rejected before it can be saved, not
// only once it is run.
func TestValidateFlowGraph_RejectsMissingAgent(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "n1"},
			{ID: "n1", Type: orchestration.NodeAgent, AgentID: "AGT-gone", Prompt: "hi"},
		},
	}
	err := rt.ValidateFlowGraph(context.Background(), g)
	if err == nil {
		t.Fatal("expected an error for a missing agent, got nil")
	}
	if !strings.Contains(err.Error(), "AGT-gone") || !strings.Contains(err.Error(), "n1") {
		t.Fatalf("error should name the node and the missing agent, got: %v", err)
	}
}

// TestValidateFlowGraph_RejectsStructuralError proves ValidateFlowGraph also
// surfaces plain structural faults (Graph.Validate), not just agent-existence
// ones — it is the single entry point flow-save callers use instead of calling
// Validate and validateFlowPreconditions separately.
func TestValidateFlowGraph_RejectsStructuralError(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())

	g := orchestration.Graph{
		Start: "missing",
		Nodes: []orchestration.Node{{ID: "n1", Type: orchestration.NodeStart}},
	}
	if err := rt.ValidateFlowGraph(context.Background(), g); err == nil {
		t.Fatal("expected a structural rejection for an unresolved start node")
	}
}

func TestValidateFlowGraphRejectsDanglingNextReference(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{{ID: "start", Type: orchestration.NodeStart, Next: "thanks"}},
	}

	err := rt.ValidateFlowGraph(context.Background(), g)
	if err == nil {
		t.Fatal("expected dangling next reference to be rejected")
	}
	if !strings.Contains(err.Error(), `node "start" field next references unknown node "thanks"`) {
		t.Fatalf("error should name node, field, and dangling target, got: %v", err)
	}
}

func TestValidateFlowGraphReportsAllDanglingReferences(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "missing-next"},
			{ID: "branch", Type: orchestration.NodeBranch, Branches: []orchestration.Branch{{Next: "missing-branch"}}},
		},
	}

	err := rt.ValidateFlowGraph(context.Background(), g)
	if err == nil {
		t.Fatal("expected dangling references to be rejected")
	}
	for _, want := range []string{"missing-next", "missing-branch"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should report dangling target %q, got: %v", want, err)
		}
	}
}

func TestValidateFlowGraphValidReferencesPass(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	g := orchestration.Graph{
		Start: "start",
		Nodes: []orchestration.Node{
			{ID: "start", Type: orchestration.NodeStart, Next: "transform"},
			{ID: "transform", Type: orchestration.NodeTransform, Template: "{{last}}", Next: "end"},
			{ID: "end", Type: orchestration.NodeEnd},
		},
	}

	if err := rt.ValidateFlowGraph(context.Background(), g); err != nil {
		t.Fatalf("expected valid graph to pass unchanged, got: %v", err)
	}
}

func TestFlowPrecheckRequiresExactAgentID(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	a := newFlowAgent(t, rt, "reviewer")

	tests := []struct {
		name    string
		nodeID  string
		agentID string
	}{
		{name: "agent name", nodeID: "review", agentID: a.Name},
		{name: "node id", nodeID: "review", agentID: "review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := orchestration.Graph{
				Start: tt.nodeID,
				Nodes: []orchestration.Node{{
					ID: tt.nodeID, Type: orchestration.NodeAgent, AgentID: tt.agentID, Prompt: "hi",
				}},
			}
			err := rt.validateFlowPreconditions(context.Background(), g)
			if err == nil {
				t.Fatalf("expected agentId %q to be rejected", tt.agentID)
			}
			if !strings.Contains(err.Error(), "existing agent ID") {
				t.Fatalf("error should require an existing agent ID, got: %v", err)
			}
		})
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

func TestRunFlowRejectsNonIDAgentReferencesBeforeCreatingRun(t *testing.T) {
	tests := []struct {
		name string
		ref  func(db.Agent) string
	}{
		{name: "agent name", ref: func(a db.Agent) string { return a.Name }},
		{name: "node id", ref: func(db.Agent) string { return "review" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newTestRuntime(t, t.TempDir())
			ctx := context.Background()
			a := newFlowAgent(t, rt, "reviewer")
			flowID := createFlow(t, rt, orchestration.Graph{
				Start: "review",
				Nodes: []orchestration.Node{{
					ID: "review", Type: orchestration.NodeAgent, AgentID: tt.ref(a), Prompt: "hi",
				}},
			})

			if _, err := rt.RunFlow(ctx, flowID, "input", false, nil); err == nil {
				t.Fatalf("expected RunFlow to reject agentId %q", tt.ref(a))
			}
			runs, err := rt.db.ListFlowRuns(ctx, flowID)
			if err != nil {
				t.Fatalf("list flow runs: %v", err)
			}
			if len(runs) != 0 {
				t.Fatalf("expected no FlowRun row for rejected agentId %q, got %d", tt.ref(a), len(runs))
			}
		})
	}
}

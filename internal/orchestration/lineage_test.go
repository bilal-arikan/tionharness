package orchestration

import (
	"context"
	"testing"
)

// nodeIDCapturingRunner records the NodeID carried on the context of every child
// flow call, which is how the agent layer attributes a child run to the exact
// node that launched it.
type nodeIDCapturingRunner struct {
	okRunner
	subflowNodeID string
	spawnNodeID   string
}

func (c *nodeIDCapturingRunner) RunChildFlow(ctx context.Context, flowID, _ string) (string, error) {
	c.subflowNodeID = NodeIDFromContext(ctx)
	return "child:" + flowID, nil
}

func (c *nodeIDCapturingRunner) SpawnChildFlows(ctx context.Context, flowIDs []string, _ string) ([]string, error) {
	c.spawnNodeID = NodeIDFromContext(ctx)
	ids := make([]string, len(flowIDs))
	for i, f := range flowIDs {
		ids[i] = "RUN-" + f
	}
	return ids, nil
}

func (c *nodeIDCapturingRunner) JoinChildFlows(_ context.Context, runIDs []string, _ int, _ bool, _ func(int, int)) ([]string, error) {
	return runIDs, nil
}

// TestSubflow_TagsLaunchingNodeID verifies a subflow node tags the context with
// its OWN id before invoking the child runner. Without this a graph with several
// subflow nodes could not tell which node a given child run belongs to.
func TestSubflow_TagsLaunchingNodeID(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "sub-b"},
			{ID: "sub-b", Type: NodeSubflow, FlowRef: "FLW-child", Next: ""},
		},
	}
	r := &nodeIDCapturingRunner{}
	if _, err := NewEngine(r).Run(context.Background(), g, "X", NewState(g), nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.subflowNodeID != "sub-b" {
		t.Errorf("subflow child ran with node id %q, want %q", r.subflowNodeID, "sub-b")
	}
}

// TestSpawn_TagsLaunchingNodeID verifies a spawn node likewise tags its own id,
// so every child it launches is attributed to that node.
func TestSpawn_TagsLaunchingNodeID(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "fan"},
			{ID: "fan", Type: NodeSpawn, SpawnFlows: []string{"FLW-a", "FLW-b"}, Next: "wait"},
			{ID: "wait", Type: NodeJoin, SpawnRef: "fan", Next: ""},
		},
	}
	r := &nodeIDCapturingRunner{}
	if _, err := NewEngine(r).Run(context.Background(), g, "X", NewState(g), nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.spawnNodeID != "fan" {
		t.Errorf("spawn ran with node id %q, want %q", r.spawnNodeID, "fan")
	}
}

// TestAgentNode_DoesNotLeakNodeIDToSibling verifies the node-id tag is scoped to
// the node being executed: a subflow node must not observe the id of the agent
// node that ran before it.
func TestAgentNode_DoesNotLeakNodeIDToSibling(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "ag"},
			{ID: "ag", Type: NodeAgent, AgentID: "a", Prompt: "p", Next: "sub"},
			{ID: "sub", Type: NodeSubflow, FlowRef: "FLW-child", Next: ""},
		},
	}
	r := &nodeIDCapturingRunner{}
	if _, err := NewEngine(r).Run(context.Background(), g, "X", NewState(g), nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if r.subflowNodeID != "sub" {
		t.Errorf("subflow saw node id %q, want %q", r.subflowNodeID, "sub")
	}
}

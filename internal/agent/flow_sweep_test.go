package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// TestSweepWaitingFlows_FailsTimedOut verifies the await-input timeout sweeper:
// a waiting run whose await node has a TimeoutSec is failed once the deadline
// passes, while one with no timeout (or not yet expired) is left waiting.
func TestSweepWaitingFlows_FailsTimedOut(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a := newFlowAgent(t, rt, "worker")

	// Flow with a 10s-timeout await node.
	g := orchestration.Graph{
		Start: "x",
		Nodes: []orchestration.Node{
			{ID: "x", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "{{input}}", Next: "w"},
			{ID: "w", Type: orchestration.NodeAwaitInput, TimeoutSec: 10, Next: ""},
		},
	}
	flowID := createFlow(t, rt, g)

	// A run suspended at the await node "w".
	run, err := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flowID, Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.db.MarkFlowRunWaiting(ctx, run.ID, `{"current":"w","waitingAt":"w"}`); err != nil {
		t.Fatal(err)
	}
	suspendedAt := func() int64 {
		r, _ := rt.db.GetFlowRun(ctx, run.ID)
		return r.UpdatedAt
	}()

	// Sweep BEFORE the deadline → still waiting.
	rt.sweepWaitingFlowsAt(ctx, suspendedAt+5)
	if r, _ := rt.db.GetFlowRun(ctx, run.ID); r.Status != db.FlowWaiting {
		t.Fatalf("run should still be waiting before timeout, got %q", r.Status)
	}

	// Sweep AFTER the deadline → failed with a timeout error.
	rt.sweepWaitingFlowsAt(ctx, suspendedAt+15)
	r, _ := rt.db.GetFlowRun(ctx, run.ID)
	if r.Status != db.FlowFailure {
		t.Fatalf("run should be failed after timeout, got %q", r.Status)
	}
	if r.Error == "" {
		t.Errorf("timed-out run should carry a timeout error message")
	}
}

// TestSweepWaitingFlows_IgnoresNoTimeout verifies a waiting run whose await node
// has TimeoutSec=0 is never swept (waits forever).
func TestSweepWaitingFlows_IgnoresNoTimeout(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	ctx := context.Background()
	a := newFlowAgent(t, rt, "worker")
	g := orchestration.Graph{
		Start: "w",
		Nodes: []orchestration.Node{
			{ID: "w", Type: orchestration.NodeAwaitInput, Next: ""}, // TimeoutSec = 0
			{ID: "x", Type: orchestration.NodeAgent, AgentID: a.ID, Prompt: "x"},
		},
	}
	flowID := createFlow(t, rt, g)
	run, _ := rt.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flowID})
	_ = rt.db.MarkFlowRunWaiting(ctx, run.ID, `{"waitingAt":"w"}`)

	rt.sweepWaitingFlowsAt(ctx, 1<<40) // far future
	if r, _ := rt.db.GetFlowRun(ctx, run.ID); r.Status != db.FlowWaiting {
		t.Fatalf("a no-timeout await run must never be swept, got %q", r.Status)
	}
}

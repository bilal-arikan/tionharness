package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeCoordinator is an AgentRunner (via okRunner) that also implements
// CoordinatorRunner, capturing what the engine passed it.
type fakeCoordinator struct {
	okRunner
	out       string
	err       error
	got       CoordinatorSpec
	gotNodeID string
}

func (f *fakeCoordinator) RunCoordinatorNode(ctx context.Context, spec CoordinatorSpec) (string, error) {
	f.got = spec
	f.gotNodeID = NodeIDFromContext(ctx)
	return f.out, f.err
}

// panicCoordinator implements CoordinatorRunner by panicking, to verify the
// engine converts that into a clean flow error.
type panicCoordinator struct{ okRunner }

func (panicCoordinator) RunCoordinatorNode(context.Context, CoordinatorSpec) (string, error) {
	panic("coordinator boom")
}

func coordinatorGraph() Graph {
	return Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "co"},
			{ID: "co", Type: NodeCoordinator, AgentID: "AGT1", Prompt: "plan: {{input}}", Workflow: "fanout-synth", MaxTurns: 7, TimeoutSec: 90, Next: "end"},
			{ID: "end", Type: NodeEnd},
		},
	}
}

// TestCoordinatorNode_HappyPath verifies the node renders its prompt, forwards
// the node's knobs to the runner, and publishes the coordinator's reply as the
// node output (and thus {{last}}).
func TestCoordinatorNode_HappyPath(t *testing.T) {
	g := coordinatorGraph()
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	fc := &fakeCoordinator{out: "3 worker çalıştı, rapor hazır"}
	eng := NewEngine(fc)

	final, err := eng.Run(context.Background(), g, "repoyu tara", NewState(g), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.got.AgentID != "AGT1" {
		t.Errorf("agentID = %q, want AGT1", fc.got.AgentID)
	}
	if fc.got.Prompt != "plan: repoyu tara" {
		t.Errorf("prompt = %q, want the rendered template", fc.got.Prompt)
	}
	if fc.got.Workflow != "fanout-synth" {
		t.Errorf("workflow = %q, want the node's recipe slug", fc.got.Workflow)
	}
	if fc.got.MaxTurns != 7 || fc.got.TimeoutSec != 90 {
		t.Errorf("knobs = (%d, %d), want (7, 90)", fc.got.MaxTurns, fc.got.TimeoutSec)
	}
	if fc.gotNodeID != "co" {
		t.Errorf("node id on ctx = %q, want co", fc.gotNodeID)
	}
	if final.Last != fc.out {
		t.Errorf("Last = %q, want the coordinator reply", final.Last)
	}
	if final.Outputs["co"] != fc.out {
		t.Errorf("Outputs[co] = %q, want the coordinator reply", final.Outputs["co"])
	}
}

// TestCoordinatorNode_TraceCarriesPrompt verifies the trace records the rendered
// prompt as Input, so the run inspector can show input→output for the node.
func TestCoordinatorNode_TraceCarriesPrompt(t *testing.T) {
	g := coordinatorGraph()
	eng := NewEngine(&fakeCoordinator{out: "done"})

	final, err := eng.Run(context.Background(), g, "x", NewState(g), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var found bool
	for _, tr := range final.Trace {
		if tr.NodeID != "co" {
			continue
		}
		found = true
		if tr.Input != "plan: x" {
			t.Errorf("trace input = %q, want the rendered prompt", tr.Input)
		}
		if tr.Output != "done" {
			t.Errorf("trace output = %q, want done", tr.Output)
		}
	}
	if !found {
		t.Error("no trace entry for the coordinator node")
	}
}

// TestCoordinatorNode_AccumulateFolds verifies an accumulate-mode graph folds the
// coordinator result into the shared thread as ONE user/assistant pair, keeping
// the thread alternating and assistant-terminated for the next agent node.
func TestCoordinatorNode_AccumulateFolds(t *testing.T) {
	g := coordinatorGraph()
	g.Accumulate = true
	eng := NewEngine(&fakeCoordinator{out: "sonuç"})

	final, err := eng.Run(context.Background(), g, "x", NewState(g), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(final.Thread) != 2 {
		t.Fatalf("thread length = %d, want 2 (one folded pair)", len(final.Thread))
	}
	if final.Thread[0].Role != "user" || !strings.Contains(final.Thread[0].Text, "coordinator") {
		t.Errorf("fold marker = %+v, want a coordinator user turn", final.Thread[0])
	}
	if final.Thread[1].Role != "assistant" || final.Thread[1].Text != "sonuç" {
		t.Errorf("folded reply = %+v, want the coordinator output", final.Thread[1])
	}
}

// TestCoordinatorNode_UnwiredRunnerFails verifies a runner without
// CoordinatorRunner fails the node clearly instead of silently passing through.
func TestCoordinatorNode_UnwiredRunnerFails(t *testing.T) {
	g := coordinatorGraph()
	eng := NewEngine(okRunner{})

	if _, err := eng.Run(context.Background(), g, "x", NewState(g), nil); err == nil {
		t.Fatal("want an error from a runner that does not support coordinator nodes")
	} else if !strings.Contains(err.Error(), "coordinator") {
		t.Errorf("error = %v, want it to name the coordinator node", err)
	}
}

// TestCoordinatorNode_RecoversPanic verifies a panicking coordinator runner ends
// the flow with an error rather than crashing the process.
func TestCoordinatorNode_RecoversPanic(t *testing.T) {
	g := coordinatorGraph()
	eng := NewEngine(panicCoordinator{})

	_, err := eng.Run(context.Background(), g, "x", NewState(g), nil)
	if err == nil {
		t.Fatal("want an error from a panicking coordinator node")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error = %v, want it to report the panic", err)
	}
}

// TestCoordinatorNode_ErrorPropagates verifies a runner error fails the run and
// names the node.
func TestCoordinatorNode_ErrorPropagates(t *testing.T) {
	g := coordinatorGraph()
	eng := NewEngine(&fakeCoordinator{err: errors.New("settle timeout")})

	_, err := eng.Run(context.Background(), g, "x", NewState(g), nil)
	if err == nil {
		t.Fatal("want the runner error to propagate")
	}
	if !strings.Contains(err.Error(), "settle timeout") || !strings.Contains(err.Error(), `"co"`) {
		t.Errorf("error = %v, want the node id and the cause", err)
	}
}

// TestCoordinatorNode_ValidateRequiresAgent verifies a coordinator node without
// an agent is rejected structurally (like an agent node).
func TestCoordinatorNode_ValidateRequiresAgent(t *testing.T) {
	g := coordinatorGraph()
	for i := range g.Nodes {
		if g.Nodes[i].ID == "co" {
			g.Nodes[i].AgentID = ""
		}
	}
	if err := g.Validate(); err == nil {
		t.Fatal("want validation to reject a coordinator node with no agentId")
	}
}

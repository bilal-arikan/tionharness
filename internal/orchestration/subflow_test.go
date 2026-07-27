package orchestration

import (
	"context"
	"strings"
	"testing"
)

// childRunner records the child flow calls and echoes a canned output.
type childRunner struct {
	okRunner
	calls []struct{ flowID, input string }
}

func (c *childRunner) RunChildFlow(_ context.Context, flowID, input string) (string, error) {
	c.calls = append(c.calls, struct{ flowID, input string }{flowID, input})
	return "child(" + flowID + "):" + input, nil
}

// TestSubflow_RunsChildAndCapturesOutput verifies a subflow node runs its FlowRef
// child with the rendered input and captures the child's output as {{last}}.
func TestSubflow_RunsChildAndCapturesOutput(t *testing.T) {
	g := Graph{
		Start: "a",
		Nodes: []Node{
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "seed", Next: "s"},
			{ID: "s", Type: NodeSubflow, FlowRef: "FLW-child", Template: "in:{{last}}", Next: "b"},
			{ID: "b", Type: NodeTransform, Template: "final:{{last}}", Next: ""},
		},
	}
	r := &childRunner{}
	eng := NewEngine(r)
	st, err := eng.Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 child flow call, got %d", len(r.calls))
	}
	// The subflow input is rendered ({{last}} = node a's "out:seed").
	if r.calls[0].flowID != "FLW-child" || r.calls[0].input != "in:out:seed" {
		t.Errorf("child call wrong: %+v", r.calls[0])
	}
	// The child's output rides {{last}} into the final transform.
	if got := st.Outputs["b"]; got != "final:child(FLW-child):in:out:seed" {
		t.Errorf("subflow output not captured downstream, got %q", got)
	}
}

// TestSubflow_UnwiredRunnerErrors verifies a subflow node fails clearly when the
// runner cannot run child flows.
func TestSubflow_UnwiredRunnerErrors(t *testing.T) {
	g := Graph{
		Start: "s",
		Nodes: []Node{{ID: "s", Type: NodeSubflow, FlowRef: "FLW-child", Next: ""}},
	}
	// okRunner does NOT implement ChildFlowRunner.
	eng := NewEngine(okRunner{})
	if _, err := eng.Run(context.Background(), g, "X", NewState(g), nil); err == nil || !strings.Contains(err.Error(), "child flows") {
		t.Fatalf("expected an unwired-runner error, got %v", err)
	}
}

// TestValidate_SubflowRequiresFlowRef verifies a subflow node without a flowRef is
// rejected.
func TestValidate_SubflowRequiresFlowRef(t *testing.T) {
	g := Graph{Start: "start", Nodes: []Node{
		{ID: "start", Type: NodeStart, Next: "s"},
		{ID: "s", Type: NodeSubflow},
	}}
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "flowRef") {
		t.Fatalf("expected a missing-flowRef rejection, got %v", err)
	}
}

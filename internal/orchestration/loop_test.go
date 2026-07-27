package orchestration

import (
	"context"
	"strings"
	"testing"
)

// iterRunner echoes the rendered prompt, so tests can read back {{iteration}}.
type iterRunner struct{ prompts []string }

func (r *iterRunner) RunAgentNode(_ context.Context, _ string, prompt string) (string, error) {
	r.prompts = append(r.prompts, prompt)
	return "out:" + prompt, nil
}

// loopGraph builds a loop whose single-node body echoes {{iteration}}, exiting by
// the given MaxIters / Until.
func loopGraph(maxIters int, until, untilMode string) Graph {
	return Graph{
		Start: "lp",
		Nodes: []Node{
			{ID: "lp", Type: NodeLoop, Body: "body", LoopNext: "fin",
				MaxIters: maxIters, Until: until, UntilMode: untilMode},
			{ID: "body", Type: NodeAgent, AgentID: "ag", Prompt: "iter {{iteration}}", Next: ""},
			{ID: "fin", Type: NodeTransform, Template: "done:{{last}}", Next: ""},
		},
	}
}

// TestLoop_MaxItersExit verifies a loop runs exactly MaxIters body passes,
// exposes {{iteration}} 0-based, then continues at LoopNext.
func TestLoop_MaxItersExit(t *testing.T) {
	r := &iterRunner{}
	g := loopGraph(3, "", "")
	eng := NewEngine(r)
	st, err := eng.Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(r.prompts) != 3 {
		t.Fatalf("expected 3 body passes, got %d: %v", len(r.prompts), r.prompts)
	}
	if r.prompts[0] != "iter 0" || r.prompts[2] != "iter 2" {
		t.Errorf("{{iteration}} should be 0-based per pass, got %v", r.prompts)
	}
	if !strings.HasPrefix(st.Last, "done:") {
		t.Errorf("after the loop, LoopNext (transform) should run, got last=%q", st.Last)
	}
}

// TestLoop_UntilExit verifies a loop exits as soon as Until matches the body's
// output, before reaching MaxIters.
func TestLoop_UntilExit(t *testing.T) {
	// The body echoes "out:iter N"; Until "iter 2" (contains) matches on the 3rd
	// pass (iteration 2), so the loop should stop after 3 passes even though the
	// cap is higher.
	r := &iterRunner{}
	g := loopGraph(10, "iter 2", "contains")
	eng := NewEngine(r)
	if _, err := eng.Run(context.Background(), g, "X", NewState(g), nil); err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if len(r.prompts) != 3 {
		t.Fatalf("expected 3 passes before Until matched, got %d: %v", len(r.prompts), r.prompts)
	}
}

// TestValidate_LoopRequiresBound verifies a loop with neither a positive cap nor
// an Until is rejected, so it can never run away to the step cap.
func TestValidate_LoopRequiresBound(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "lp"},
			{ID: "lp", Type: NodeLoop, Body: "body", LoopNext: ""},
			{ID: "body", Type: NodeAgent, AgentID: "ag", Prompt: "x", Next: ""},
		},
	}
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "maxIters") {
		t.Fatalf("expected an unbounded-loop rejection, got %v", err)
	}
}

// TestValidate_LoopNeedsBody verifies a loop without a body is rejected.
func TestValidate_LoopNeedsBody(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "lp"},
			{ID: "lp", Type: NodeLoop, MaxIters: 1},
		},
	}
	if err := g.Validate(); err == nil || !strings.Contains(err.Error(), "body") {
		t.Fatalf("expected a missing-body rejection, got %v", err)
	}
}

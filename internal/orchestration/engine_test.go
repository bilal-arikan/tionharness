package orchestration

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// panicRunner panics on every node, simulating a crash inside an agent call.
type panicRunner struct{}

func (panicRunner) RunAgentNode(context.Context, string, string) (string, error) {
	panic("boom")
}

// okRunner echoes the prompt back, for the happy-path parallel join.
type okRunner struct{}

func (okRunner) RunAgentNode(_ context.Context, _ string, prompt string) (string, error) {
	return "out:" + prompt, nil
}

// errRunner always fails, for verifying error events + clean propagation.
type errRunner struct{}

func (errRunner) RunAgentNode(context.Context, string, string) (string, error) {
	return "", errors.New("kaboom")
}

// TestRunSequential_RecoversNodePanic verifies a panic in a sequential agent
// node becomes a normal flow error (and an "error" observer event) instead of
// crashing the process.
func TestRunSequential_RecoversNodePanic(t *testing.T) {
	g := Graph{
		Start: "a",
		Nodes: []Node{{ID: "a", Type: NodeAgent, AgentID: "a1", Prompt: "x"}},
	}
	eng := NewEngine(panicRunner{})
	var errored []string
	eng.SetObserver(func(ev NodeEvent) {
		if ev.Phase == "error" {
			errored = append(errored, ev.NodeID)
		}
	})

	_, err := eng.Run(context.Background(), g, "input", NewState(g), nil)
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected a panic-derived error, got %v", err)
	}
	if len(errored) != 1 || errored[0] != "a" {
		t.Errorf("expected one 'error' event for node a, got %v", errored)
	}
}

// TestRun_EmitsErrorEventOnNodeFailure verifies a failing (non-panic) node emits
// an "error" lifecycle event carrying the message, so a live UI can stop its
// spinner and show why.
func TestRun_EmitsErrorEventOnNodeFailure(t *testing.T) {
	g := Graph{
		Start: "a",
		Nodes: []Node{{ID: "a", Type: NodeAgent, AgentID: "a1", Prompt: "x"}},
	}
	eng := NewEngine(errRunner{})
	var got NodeEvent
	eng.SetObserver(func(ev NodeEvent) {
		if ev.Phase == "error" {
			got = ev
		}
	})

	if _, err := eng.Run(context.Background(), g, "in", NewState(g), nil); err == nil {
		t.Fatal("expected node failure to propagate as an error")
	}
	if got.Phase != "error" || got.NodeID != "a" || !strings.Contains(got.Error, "kaboom") {
		t.Errorf("expected an 'error' event with the message, got %+v", got)
	}
}

// TestRunParallel_RecoversChildPanic verifies a panic inside a parallel child is
// converted into a normal flow error instead of crashing the process — every
// flow runs in its own goroutine, so an unrecovered panic would take the whole
// runtime (all workspaces) down.
func TestRunParallel_RecoversChildPanic(t *testing.T) {
	g := Graph{
		Start: "p",
		Nodes: []Node{
			{ID: "p", Type: NodeParallel, Parallel: []string{"a"}},
			{ID: "a", Type: NodeAgent, AgentID: "agent-1", Prompt: "hi"},
		},
	}
	eng := NewEngine(panicRunner{})

	_, err := eng.Run(context.Background(), g, "input", NewState(g), nil)
	if err == nil {
		t.Fatal("expected an error from the panicking parallel child, got nil (did it crash or swallow?)")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error should mention the panic, got %q", err.Error())
	}
}

// verdictRunner drives a generator↔evaluator (GAN) loop: the "eval" agent emits
// REFINE for the first refineRounds evaluations, then SHIP. Every other agent
// echoes its id. Used to exercise a cyclic graph (evaluate → branch → generate).
type verdictRunner struct {
	evalCalls    int
	refineRounds int
}

func (r *verdictRunner) RunAgentNode(_ context.Context, agentID, _ string) (string, error) {
	if agentID == "eval" {
		r.evalCalls++
		if r.evalCalls > r.refineRounds {
			return "all criteria pass\nVERDICT: SHIP", nil
		}
		return "found issues\nVERDICT: REFINE", nil
	}
	return "out:" + agentID, nil
}

// ganGraph is the minimal generator↔evaluator loop: generate → evaluate →
// decide(branch), where decide routes SHIP→finalize, PIVOT→pivot, default→back to
// generate. The back edges (decide→generate, pivot→generate) make it cyclic on
// purpose — the engine allows cycles and bounds them with maxSteps.
func ganGraph() Graph {
	return Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "gen"},
			{ID: "gen", Type: NodeAgent, AgentID: "gen", Prompt: "build {{input}}", Next: "eval"},
			{ID: "eval", Type: NodeAgent, AgentID: "eval", Prompt: "judge {{node.gen}}", Next: "decide"},
			{ID: "decide", Type: NodeBranch, MatchMode: "regex", Branches: []Branch{
				{Contains: `(?m)^VERDICT:\s*SHIP`, Next: "finalize"},
				{Contains: `(?m)^VERDICT:\s*PIVOT`, Next: "pivot"},
				{Contains: "", Next: "gen"},
			}},
			{ID: "pivot", Type: NodeAgent, AgentID: "gen", Prompt: "pivot", Next: "gen"},
			{ID: "finalize", Type: NodeTransform, Template: "done:{{node.eval}}", Next: ""},
		},
	}
}

// TestValidate_AllowsCyclicGraph verifies the validator accepts a cyclic graph (a
// branch routing back to an earlier node), since iteration loops are a supported
// pattern (e.g. the generator↔evaluator GAN loop).
func TestValidate_AllowsCyclicGraph(t *testing.T) {
	if err := ganGraph().Validate(); err != nil {
		t.Fatalf("cyclic GAN graph should validate, got: %v", err)
	}
}

// TestRun_GANLoop_RefinesThenShips drives the loop through two REFINE iterations
// and a final SHIP, ending at the finalize transform node. Proves the back edge
// (decide → generate) loops and that the SHIP verdict exits the loop.
func TestRun_GANLoop_RefinesThenShips(t *testing.T) {
	g := ganGraph()
	r := &verdictRunner{refineRounds: 2}
	eng := NewEngine(r)

	final, err := eng.Run(context.Background(), g, "a button", NewState(g), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.evalCalls != 3 {
		t.Errorf("expected 3 evaluations (2 refine + 1 ship), got %d", r.evalCalls)
	}
	if final.Current != "" {
		t.Errorf("expected a finished run (Current==\"\"), got %q", final.Current)
	}
	if !strings.Contains(final.Last, "done:") || !strings.Contains(final.Last, "VERDICT: SHIP") {
		t.Errorf("expected finalize output from the SHIP branch, got %q", final.Last)
	}
}

// TestRun_GANLoop_StepCapBackstop verifies an evaluator that never ships is
// stopped by the step cap rather than looping forever — the safety backstop for
// a cyclic graph.
func TestRun_GANLoop_StepCapBackstop(t *testing.T) {
	g := ganGraph()
	r := &verdictRunner{refineRounds: 1 << 30} // never ships
	eng := NewEngine(r)

	_, err := eng.Run(context.Background(), g, "x", NewState(g), nil)
	if err == nil || !strings.Contains(err.Error(), "step cap") {
		t.Fatalf("expected a step-cap error from the runaway loop, got %v", err)
	}
}

// TestRunParallel_HappyPath is a sanity check that the recover wrapper does not
// disturb a normal parallel run.
func TestRunParallel_HappyPath(t *testing.T) {
	g := Graph{
		Start: "p",
		Nodes: []Node{
			{ID: "p", Type: NodeParallel, Parallel: []string{"a", "b"}},
			{ID: "a", Type: NodeAgent, AgentID: "a1", Prompt: "x", Title: "A"},
			{ID: "b", Type: NodeAgent, AgentID: "a2", Prompt: "y", Title: "B"},
		},
	}
	eng := NewEngine(okRunner{})

	final, err := eng.Run(context.Background(), g, "input", NewState(g), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(final.Last, "out:x") || !strings.Contains(final.Last, "out:y") {
		t.Errorf("joined output missing a child result: %q", final.Last)
	}
}

// TestRenderVars covers the template placeholders resolvable in prompts/templates:
// {{input}}, {{last}}, {{node.<id>}} plus the wall-clock date/time vars.
func TestRenderVars(t *testing.T) {
	st := NewState(Graph{Start: "a"})
	st.Last = "LAST"
	st.Outputs["a"] = "AOUT"

	got := render("in={{input}} last={{last}} a={{node.a}}", "IN", st)
	want := "in=IN last=LAST a=AOUT"
	if got != want {
		t.Fatalf("core vars: got %q want %q", got, want)
	}

	// date/time vars resolve to a non-empty, correctly shaped value.
	dt := render("{{date}} {{time}} {{datetime}}", "", st)
	if strings.Contains(dt, "{{") {
		t.Fatalf("date/time vars not substituted: %q", dt)
	}
	parts := strings.SplitN(dt, " ", 2)
	if len(parts[0]) != len("2006-01-02") {
		t.Fatalf("{{date}} wrong shape: %q", dt)
	}
}

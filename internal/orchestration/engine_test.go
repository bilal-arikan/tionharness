package orchestration

import (
	"context"
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

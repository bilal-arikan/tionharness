package orchestration

import (
	"context"
	"testing"
)

// awaitGraph is agent → await-input → agent, where the second agent echoes the
// injected input via {{last}}.
func awaitGraph() Graph {
	return Graph{
		Start: "a",
		Nodes: []Node{
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "start", Next: "w"},
			{ID: "w", Type: NodeAwaitInput, Next: "b"},
			{ID: "b", Type: NodeAgent, AgentID: "ag", Prompt: "got: {{last}}", Next: ""},
		},
	}
}

// TestAwaitInput_SuspendsThenResumes verifies the durable await-input keystone:
// Run returns (state, nil) suspended at the await node with Current parked there
// and only the pre-await node executed; re-entering with the input injected into
// Last consumes it, runs the post-await node with {{last}} = input, and finishes.
func TestAwaitInput_SuspendsThenResumes(t *testing.T) {
	eng := NewEngine(okRunner{})
	var phases []string
	eng.SetObserver(func(ev NodeEvent) {
		if ev.NodeID == "w" {
			phases = append(phases, ev.Phase)
		}
	})

	// First pass: suspends at the await node.
	st, err := eng.Run(context.Background(), awaitGraph(), "X", NewState(awaitGraph()), nil)
	if err != nil {
		t.Fatalf("suspend pass errored: %v", err)
	}
	if st.WaitingAt != "w" {
		t.Fatalf("expected WaitingAt=w, got %q", st.WaitingAt)
	}
	if st.Current != "w" {
		t.Fatalf("Current should stay parked at the await node, got %q", st.Current)
	}
	if _, ran := st.Outputs["a"]; !ran {
		t.Errorf("pre-await node a should have run")
	}
	if _, ran := st.Outputs["b"]; ran {
		t.Errorf("post-await node b must NOT run before input arrives")
	}
	if len(phases) == 0 || phases[len(phases)-1] != "waiting" {
		t.Errorf("expected a 'waiting' observer phase for the await node, got %v", phases)
	}

	// Resume: inject the delivered input into Last and re-enter from the same state.
	st.Last = "hello there"
	final, err := eng.Run(context.Background(), awaitGraph(), "X", st, nil)
	if err != nil {
		t.Fatalf("resume pass errored: %v", err)
	}
	if final.WaitingAt != "" {
		t.Fatalf("run should not be waiting after resume, got %q", final.WaitingAt)
	}
	if final.Current != "" {
		t.Fatalf("run should be terminal after resume, got Current=%q", final.Current)
	}
	if got := final.Outputs["b"]; got != "out:got: hello there" {
		t.Errorf("node b should consume the injected input via {{last}}, got %q", got)
	}
	// The await node records the received input as its output/trace.
	if got := final.Outputs["w"]; got != "hello there" {
		t.Errorf("await node should record the received input, got %q", got)
	}
}

// TestAwaitInput_ValidatesNextRef verifies the await node's Next reference is
// structurally validated like other single-exit nodes.
func TestAwaitInput_ValidatesNextRef(t *testing.T) {
	g := Graph{
		Start: "w",
		Nodes: []Node{{ID: "w", Type: NodeAwaitInput, Next: "missing"}},
	}
	if err := g.Validate(); err == nil {
		t.Fatal("expected a dangling-Next rejection for await-input")
	}
}

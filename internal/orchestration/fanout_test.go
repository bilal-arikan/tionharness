package orchestration

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// fanOutRunner records the legs each fan-out node received and echoes them back.
// It also serves plain agent nodes, so mixed graphs can run through one runner.
type fanOutRunner struct {
	mu    sync.Mutex
	calls map[string][]Leg // node id -> rendered legs
}

func (f *fanOutRunner) RunAgentNode(_ context.Context, _ string, prompt string) (string, error) {
	return "agent:" + prompt, nil
}

func (f *fanOutRunner) RunFanOutNode(ctx context.Context, legs []Leg) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string][]Leg{}
	}
	f.calls[NodeIDFromContext(ctx)] = legs
	parts := make([]string, len(legs))
	for i, l := range legs {
		parts[i] = l.Target + "=" + l.Task
	}
	return strings.Join(parts, ";"), nil
}

// TestValidateAgentNodeForms pins the widened agent-node rule: exactly one of
// agentId or legs. The single-agent form keeps its historical error message.
func TestValidateAgentNodeForms(t *testing.T) {
	graph := func(n Node) Graph {
		n.ID = "a"
		n.Type = NodeAgent
		return Graph{Start: "s", Nodes: []Node{{ID: "s", Type: NodeStart, Next: "a"}, n}}
	}
	cases := []struct {
		name    string
		node    Node
		wantErr string
	}{
		{"agentId only", Node{AgentID: "A1", Prompt: "p"}, ""},
		{"legs only", Node{Legs: []Leg{{Target: "explore", Task: "t"}}}, ""},
		{"neither", Node{Prompt: "p"}, "has no agentId"},
		{"both", Node{AgentID: "A1", Legs: []Leg{{Target: "explore", Task: "t"}}}, "both an agentId and legs"},
		{"leg without task", Node{Legs: []Leg{{Target: "explore"}}}, "has no task"},
		{"leg without target", Node{Legs: []Leg{{Task: "t"}}}, "has no target"},
		{"legs with prompt", Node{Prompt: "p", Legs: []Leg{{Target: "explore", Task: "t"}}}, "has a prompt"},
	}
	for _, c := range cases {
		err := graph(c.node).Validate()
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: want error containing %q, got %v", c.name, c.wantErr, err)
		}
	}
}

// TestFanOutNodeRunsThroughHookWithRenderedLegs: a fan-out node goes to the
// FanOutRunner hook (not RunAgentNode), its leg tasks are rendered against the
// state, and it yields ONE output and ONE trace entry.
func TestFanOutNodeRunsThroughHookWithRenderedLegs(t *testing.T) {
	g := Graph{Start: "s", Nodes: []Node{
		{ID: "s", Type: NodeStart, Next: "first"},
		{ID: "first", Type: NodeAgent, AgentID: "A1", Prompt: "{{input}}", Next: "fan"},
		{ID: "fan", Type: NodeAgent, Legs: []Leg{
			{Target: "explore", Task: "check {{last}}"},
			{Target: "reviewer", Task: "review {{node.first}}"},
		}},
	}}
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	r := &fanOutRunner{}
	st, err := NewEngine(r).Run(context.Background(), g, "in", NewState(g), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	legs := r.calls["fan"]
	if len(legs) != 2 || legs[0].Task != "check agent:in" || legs[1].Task != "review agent:in" {
		t.Fatalf("legs not rendered against the state: %+v", legs)
	}
	if st.Last != "explore=check agent:in;reviewer=review agent:in" {
		t.Fatalf("unexpected fan-out output %q", st.Last)
	}
	entries := 0
	for _, tr := range st.Trace {
		if tr.NodeID == "fan" {
			entries++
			if !strings.Contains(tr.Input, "[1] explore: check agent:in") {
				t.Errorf("trace input should digest the rendered legs, got %q", tr.Input)
			}
		}
	}
	if entries != 1 {
		t.Fatalf("a fan-out node must produce exactly one trace entry, got %d", entries)
	}
}

// TestFanOutNodeAsParallelChild: runParallel accepts a fan-out agent node as a
// child next to a single-agent child.
func TestFanOutNodeAsParallelChild(t *testing.T) {
	g := Graph{Start: "s", Nodes: []Node{
		{ID: "s", Type: NodeStart, Next: "p"},
		{ID: "p", Type: NodeParallel, Parallel: []string{"one", "fan"}},
		{ID: "one", Type: NodeAgent, AgentID: "A1", Prompt: "x"},
		{ID: "fan", Type: NodeAgent, Legs: []Leg{{Target: "explore", Task: "y"}}},
	}}
	if err := g.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	r := &fanOutRunner{}
	st, err := NewEngine(r).Run(context.Background(), g, "", NewState(g), nil)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if st.Outputs["one"] != "agent:x" || st.Outputs["fan"] != "explore=y" {
		t.Fatalf("unexpected child outputs %+v", st.Outputs)
	}
}

// TestFanOutNodeWithoutHookFailsClearly: a runner lacking the hook must fail the
// node explicitly rather than fall back to RunAgentNode with an empty agentId.
func TestFanOutNodeWithoutHookFailsClearly(t *testing.T) {
	g := Graph{Start: "s", Nodes: []Node{
		{ID: "s", Type: NodeStart, Next: "fan"},
		{ID: "fan", Type: NodeAgent, Legs: []Leg{{Target: "explore", Task: "y"}}},
	}}
	_, err := NewEngine(okRunner{}).Run(context.Background(), g, "", NewState(g), nil)
	if err == nil || !strings.Contains(err.Error(), "does not support fan-out") {
		t.Fatalf("expected a not-wired error, got %v", err)
	}
}

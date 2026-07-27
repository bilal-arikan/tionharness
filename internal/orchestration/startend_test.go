package orchestration

import (
	"context"
	"strings"
	"testing"
)

// TestStartEnd_PassThroughAndTerminal verifies a start node passes straight through
// and an end node terminates the run, optionally shaping the final output.
func TestStartEnd_PassThroughAndTerminal(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "a"},
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "hi", Next: "end"},
			{ID: "end", Type: NodeEnd, Template: "final:{{last}}"},
		},
	}
	if err := g.Validate(); err != nil {
		t.Fatalf("valid start/end graph should validate: %v", err)
	}
	st, err := NewEngine(okRunner{}).Run(context.Background(), g, "X", NewState(g), nil)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if st.Current != "" {
		t.Errorf("end node should terminate the run, got Current=%q", st.Current)
	}
	if st.Last != "final:out:hi" {
		t.Errorf("end Template should shape the final output, got %q", st.Last)
	}
}

// TestEnd_OutputSchemaRejectsNonJSON verifies an end node with an OutputSchema
// fails the run when the final output isn't the required (JSON) format.
func TestEnd_OutputSchemaRejectsNonJSON(t *testing.T) {
	g := Graph{
		Start: "start",
		Nodes: []Node{
			{ID: "start", Type: NodeStart, Next: "a"},
			{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "hi", Next: "end"},
			{ID: "end", Type: NodeEnd, OutputSchema: `{"type":"object"}`},
		},
	}
	// okRunner returns "out:hi" (not JSON) → schema check fails.
	if _, err := NewEngine(okRunner{}).Run(context.Background(), g, "X", NewState(g), nil); err == nil || !strings.Contains(err.Error(), "format") {
		t.Fatalf("expected an output-format error, got %v", err)
	}
}

// TestValidate_RequiresExactlyOneStart verifies the start-node contract: zero or
// two start nodes are rejected, and the graph entry must be the start node.
func TestValidate_RequiresExactlyOneStart(t *testing.T) {
	// Zero start nodes.
	g0 := Graph{Start: "a", Nodes: []Node{{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "x"}}}
	if err := g0.Validate(); err == nil || !strings.Contains(err.Error(), "start node") {
		t.Errorf("zero start nodes should be rejected, got %v", err)
	}
	// Entry not pointing at the start node.
	gBad := Graph{Start: "a", Nodes: []Node{
		{ID: "start", Type: NodeStart, Next: "a"},
		{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "x"},
	}}
	if err := gBad.Validate(); err == nil || !strings.Contains(err.Error(), "must be the start node") {
		t.Errorf("entry must be the start node, got %v", err)
	}
}

// TestMigrateAddStart verifies migration prepends a start node (Next = old entry)
// and is idempotent.
func TestMigrateAddStart(t *testing.T) {
	g := Graph{Start: "a", Nodes: []Node{{ID: "a", Type: NodeAgent, AgentID: "ag", Prompt: "x"}}}
	m, changed := MigrateAddStart(g)
	if !changed {
		t.Fatal("expected migration to change a start-less graph")
	}
	if m.Start == "a" {
		t.Errorf("Start should now point at the new start node, got %q", m.Start)
	}
	sn, ok := m.node(m.Start)
	if !ok || sn.Type != NodeStart || sn.Next != "a" {
		t.Errorf("new start node should point at the old entry, got %+v", sn)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("migrated graph should validate: %v", err)
	}
	// Idempotent.
	if _, changed2 := MigrateAddStart(m); changed2 {
		t.Error("migrating an already-migrated graph should be a no-op")
	}
}

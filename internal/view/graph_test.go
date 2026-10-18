package view

import (
	"context"
	"encoding/json"
	"testing"
)

// TestGraphWalksWholeTreeOnce pins the whole-map contract: root + the buckets
// + every member, agent -> session edges, and a session reached through two
// parents appears once with both edges.
func TestGraphWalksWholeTreeOnce(t *testing.T) {
	ctx := context.Background()
	p := childrenFixture()

	g, err := p.Graph(ctx)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	root := Ref{Kind: KindSpace, ID: WorkspaceRefID}
	if !hasHandleRef(g.Nodes, root) {
		t.Fatalf("root missing: %+v", g.Nodes)
	}
	for _, bucket := range workspaceChildren() {
		if !hasEdge(g.Edges, root, bucket.Ref) {
			t.Errorf("root -> %s edge missing", bucket.Ref)
		}
	}

	sessions := Ref{Kind: KindCategory, ID: CategorySessions}
	kindGroup := Ref{Kind: KindCategory, ID: "skind:other"}
	agent := Ref{Kind: KindAgent, ID: "AG1"}
	s0 := Ref{Kind: KindSession, ID: "S0"}
	for _, want := range []GraphEdge{
		{sessions, kindGroup}, {kindGroup, s0}, {agent, s0}, // bucket -> kind group -> session; multi-parent
	} {
		if !hasEdge(g.Edges, want.Source, want.Target) {
			t.Errorf("edge %s -> %s missing", want.Source, want.Target)
		}
	}
	if n := countNodes(g.Nodes, s0); n != 1 {
		t.Errorf("S0 appears %d times, want once", n)
	}
	// Sessions are leaves: no edge leaves a session node.
	for _, e := range g.Edges {
		if e.Source.Kind == KindSession {
			t.Errorf("session must be a leaf, got edge %s -> %s", e.Source, e.Target)
		}
	}

	// Deterministic order: nodes and edges sorted by Ref.String.
	for i := 1; i < len(g.Nodes); i++ {
		if g.Nodes[i-1].Ref.String() >= g.Nodes[i].Ref.String() {
			t.Fatalf("nodes not sorted at %d: %s >= %s", i, g.Nodes[i-1].Ref, g.Nodes[i].Ref)
		}
	}
	for i := 1; i < len(g.Edges); i++ {
		a, b := g.Edges[i-1], g.Edges[i]
		if a.Source.String() > b.Source.String() ||
			(a.Source == b.Source && a.Target.String() >= b.Target.String()) {
			t.Fatalf("edges not sorted at %d", i)
		}
	}
	// Every edge endpoint is a node.
	for _, e := range g.Edges {
		if !hasHandleRef(g.Nodes, e.Source) || !hasHandleRef(g.Nodes, e.Target) {
			t.Errorf("dangling edge %s -> %s", e.Source, e.Target)
		}
	}

	// JSON shape the frontend relies on: nested ref objects, never strings.
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded struct {
		Nodes []struct {
			Label string `json:"label"`
			Ref   Ref    `json:"ref"`
		} `json:"nodes"`
		Edges []struct {
			Source Ref `json:"source"`
			Target Ref `json:"target"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(decoded.Nodes) != len(g.Nodes) || len(decoded.Edges) != len(g.Edges) {
		t.Fatalf("json roundtrip lost entries: %s", raw)
	}
}

// TestGraphSurvivesMissingOptionalSources: without a skill catalog / findings
// store the map still renders; those buckets are simply empty.
func TestGraphSurvivesMissingOptionalSources(t *testing.T) {
	p := NewProjector(&fakeStore{})
	g, err := p.Graph(context.Background())
	if err != nil {
		t.Fatalf("graph without sources: %v", err)
	}
	if len(g.Nodes) != 12 { // root + 11 buckets
		t.Fatalf("nodes=%d, want 12: %+v", len(g.Nodes), g.Nodes)
	}
	if len(g.Edges) != 11 {
		t.Fatalf("edges=%d, want 11", len(g.Edges))
	}
}

func TestGraphNoStore(t *testing.T) {
	if _, err := (&Projector{}).Graph(context.Background()); err == nil {
		t.Fatal("expected error without store")
	}
}

func hasEdge(edges []GraphEdge, source, target Ref) bool {
	for _, e := range edges {
		if e.Source == source && e.Target == target {
			return true
		}
	}
	return false
}

func countNodes(nodes []Handle, ref Ref) int {
	n := 0
	for _, h := range nodes {
		if h.Ref == ref {
			n++
		}
	}
	return n
}

package view

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// TestGraphWalksWholeTreeOnceAndTerminatesCycles pins the whole-map contract:
// root + eleven buckets + every member, agent -> session edges, coordinator
// cycles and self-loops do not recurse, and a session reached through two
// parents appears once with both edges.
func TestGraphWalksWholeTreeOnceAndTerminatesCycles(t *testing.T) {
	ctx := context.Background()
	p := childrenFixture()
	// Cycle + self-loop on top of the shared fixture.
	store := p.store.(*fakeStore)
	now := time.Now().Unix()
	store.sessions = append(store.sessions,
		db.Session{ID: "CYC-A", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "CYC-B"},
		db.Session{ID: "CYC-B", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "CYC-A"},
		db.Session{ID: "SELF", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "SELF"},
	)

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
	cycA := Ref{Kind: KindSession, ID: "CYC-A"}
	cycB := Ref{Kind: KindSession, ID: "CYC-B"}
	self := Ref{Kind: KindSession, ID: "SELF"}
	for _, want := range []GraphEdge{
		{sessions, kindGroup}, {kindGroup, cycA}, {agent, cycA}, // bucket -> kind group -> session; multi-parent
		{cycA, cycB}, {cycB, cycA}, // cycle
		{self, self}, // self-loop
	} {
		if !hasEdge(g.Edges, want.Source, want.Target) {
			t.Errorf("edge %s -> %s missing", want.Source, want.Target)
		}
	}
	if n := countNodes(g.Nodes, cycA); n != 1 {
		t.Errorf("CYC-A appears %d times, want once", n)
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

// TestGraphIncludesTrajectoryEdges: a root session drills into its Rota and the
// Rota into the entities its nodes bind, exactly as Children does.
func TestGraphIncludesTrajectoryEdges(t *testing.T) {
	ctx := context.Background()
	now := time.Now().Unix()
	p := NewProjector(&fakeStore{
		agents: []db.Agent{{ID: "AG1", Name: "builder"}},
		sessions: []db.Session{
			{ID: "COORD", AgentID: "AG1", UpdatedAt: now, CoordinatorMode: true},
			{ID: "W1", AgentID: "AG1", UpdatedAt: now, CoordinatorSessionID: "COORD"},
		},
		trajectories: []db.Trajectory{sampleTrajectory()},
	})
	g, err := p.Graph(ctx)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	var traj *Handle
	for i := range g.Nodes {
		if g.Nodes[i].Ref.Kind == KindTrajectory {
			traj = &g.Nodes[i]
			break
		}
	}
	if traj == nil {
		t.Fatalf("trajectory node missing: %+v", g.Nodes)
	}
	parents, children := 0, 0
	for _, e := range g.Edges {
		if e.Target == traj.Ref && e.Source.Kind == KindSession {
			parents++
		}
		if e.Source == traj.Ref {
			children++
		}
	}
	if parents == 0 {
		t.Errorf("no session -> trajectory edge: %+v", g.Edges)
	}
	if children == 0 {
		t.Errorf("no trajectory -> bound entity edge: %+v", g.Edges)
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

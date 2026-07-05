package memory

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestGraphLinksSimilarMemories verifies the knowledge graph links memories with
// overlapping terms while leaving an unrelated memory isolated, and that the
// threshold gates edge creation.
func TestGraphLinksSimilarMemories(t *testing.T) {
	ctx := context.Background()
	s, d, agentID := newTestStore(t)
	defer d.Close()

	// Two memories share vocabulary; the third is unrelated.
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, "golang concurrency goroutine channel scheduler"); err != nil {
		t.Fatalf("remember a: %v", err)
	}
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, "goroutine channel scheduler runtime golang"); err != nil {
		t.Fatalf("remember b: %v", err)
	}
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, "strawberry banana smoothie recipe kitchen"); err != nil {
		t.Fatalf("remember c: %v", err)
	}

	g, err := s.Graph(ctx, agentID, 0.2, 100)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if len(g.Nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(g.Nodes))
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected exactly 1 edge between the two similar memories, got %d", len(g.Edges))
	}

	// Degree: the two linked nodes have degree 1, the unrelated one degree 0.
	linked := 0
	for _, n := range g.Nodes {
		if n.Degree > 0 {
			linked++
		}
	}
	if linked != 2 {
		t.Fatalf("expected 2 connected nodes, got %d", linked)
	}

	// A very high threshold drops the edge.
	g2, err := s.Graph(ctx, agentID, 0.99, 100)
	if err != nil {
		t.Fatalf("graph high threshold: %v", err)
	}
	if len(g2.Edges) != 0 {
		t.Fatalf("expected no edges at threshold 0.99, got %d", len(g2.Edges))
	}

	// maxEdges caps the result.
	g3, err := s.Graph(ctx, agentID, 0.2, 0) // 0 → default cap, still 1 edge here
	if err != nil {
		t.Fatalf("graph default cap: %v", err)
	}
	if len(g3.Edges) != 1 {
		t.Fatalf("expected 1 edge with default cap, got %d", len(g3.Edges))
	}
}

// TestGraphEdgeScoreEqualsCosine verifies the edge weight carries the actual
// lexical-cosine similarity (kept ≥ threshold and ≤ 1), not just a boolean link.
// The pre-existing test only counted edges; the documented "kenar ağırlığı =
// benzerlik skoru" contract (_Docs/23-ILISKI-GRAFIGI.md) had no assertion.
func TestGraphEdgeScoreEqualsCosine(t *testing.T) {
	ctx := context.Background()
	s, d, agentID := newTestStore(t)
	defer d.Close()

	const (
		a = "golang concurrency goroutine channel scheduler"
		b = "goroutine channel scheduler runtime golang"
	)
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, a); err != nil {
		t.Fatalf("remember a: %v", err)
	}
	if _, err := s.Remember(ctx, agentID, db.MemoryDocument, b); err != nil {
		t.Fatalf("remember b: %v", err)
	}

	const threshold = 0.2
	g, err := s.Graph(ctx, agentID, threshold, 100)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges))
	}

	va, vb := buildVector(a), buildVector(b)
	want := cosineNorm(va, vb, norm(va))
	got := g.Edges[0].Score
	if got < threshold || got > 1 {
		t.Errorf("edge score %v outside [%v,1]", got, threshold)
	}
	const eps = 1e-9
	if diff := got - want; diff > eps || diff < -eps {
		t.Errorf("edge score = %v, want cosine %v", got, want)
	}
}

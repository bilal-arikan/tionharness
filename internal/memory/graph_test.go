package memory

import (
	"context"
	"testing"

	"github.com/bilal/swarmgo/internal/db"
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

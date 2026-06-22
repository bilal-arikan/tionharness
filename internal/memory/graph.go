package memory

import (
	"context"
	"sort"
)

// GraphNode is one memory rendered as a node in the knowledge graph.
type GraphNode struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Content   string `json:"content"`
	CreatedAt int64  `json:"createdAt"`
	Degree    int    `json:"degree"` // number of edges incident to this node
}

// GraphEdge is a similarity link between two memories, weighted by lexical
// cosine in [0,1].
type GraphEdge struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Score  float64 `json:"score"`
}

// Graph is an agent's memory similarity graph: every memory is a node and pairs
// whose cosine similarity meets the threshold are linked.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph builds the knowledge graph for an agent: pairwise lexical-cosine
// similarity between all of its memories, keeping only edges at or above
// `threshold` (clamped to a sane floor) and capping the total to `maxEdges`
// strongest links so a dense store doesn't explode the payload. Node degree is
// computed from the kept edges so the frontend can size/rank hubs.
func (s *Store) Graph(ctx context.Context, agentID string, threshold float64, maxEdges int) (Graph, error) {
	if threshold < minScore {
		threshold = minScore
	}
	if maxEdges <= 0 {
		maxEdges = 400
	}

	// Display kinds only — the always-in-context core blocks are shown in their
	// own card, not as nodes in the similarity graph.
	sources, err := s.db.ListKnowledge(ctx, agentID, recallKinds...) // newest first
	if err != nil {
		return Graph{}, err
	}

	// Materialise each memory's term vector (backfilling rows without a cached
	// embedding) and its norm once, so the O(n²) pass below is cheap per pair.
	vecs := make([]vector, len(sources))
	norms := make([]float64, len(sources))
	for i, src := range sources {
		v := unmarshalVector(src.Embedding)
		if v == nil {
			v = buildVector(src.Content)
		}
		vecs[i] = v
		norms[i] = norm(v)
	}

	edges := make([]GraphEdge, 0)
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			score := cosineNorm(vecs[i], vecs[j], norms[i])
			if score >= threshold {
				edges = append(edges, GraphEdge{
					Source: sources[i].ID,
					Target: sources[j].ID,
					Score:  score,
				})
			}
		}
	}

	// Keep the strongest links first, then cap.
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].Score > edges[j].Score })
	if len(edges) > maxEdges {
		edges = edges[:maxEdges]
	}

	degree := make(map[string]int, len(sources))
	for _, e := range edges {
		degree[e.Source]++
		degree[e.Target]++
	}

	nodes := make([]GraphNode, len(sources))
	for i, src := range sources {
		nodes[i] = GraphNode{
			ID:        src.ID,
			Kind:      src.Kind,
			Content:   src.Content,
			CreatedAt: src.CreatedAt,
			Degree:    degree[src.ID],
		}
	}

	return Graph{Nodes: nodes, Edges: edges}, nil
}

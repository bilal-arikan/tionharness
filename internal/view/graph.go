package view

import (
	"context"
	"fmt"
	"sort"
)

// GraphEdge is one directed structural relationship in the whole-workspace map:
// Source drills into Target (the same edge Children would list).
type GraphEdge struct {
	Source Ref `json:"source"`
	Target Ref `json:"target"`
}

// Graph is the complete structural map of a workspace: every node reachable from
// the workspace root plus every parent -> child edge between them. The Explorer
// screen renders it as one force-directed network, so unlike Children it applies
// no per-node presentation cap — the honest total is what the physics lays out.
type Graph struct {
	Nodes []Handle    `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// Graph walks the structural tree from the workspace root once, breadth first,
// against a single store snapshot (structuralCache). Sessions parent worker
// sessions and coordinators can point at each other, so this is a graph, not a
// tree: a visited set keyed by Ref terminates cycles and self-loops, and an edge
// set de-duplicates a relationship discovered through two paths. Node and edge
// order is deterministic (sorted by Ref.String) so two calls over the same store
// produce byte-identical JSON.
//
// The optional projector sources (skill catalog, findings store) may be absent;
// their categories then stay as empty buckets instead of failing the whole map,
// mirroring structuralCache.nodes.
func (p *Projector) Graph(ctx context.Context) (Graph, error) {
	if p == nil || p.store == nil {
		return Graph{}, fmt.Errorf("view: projector has no store")
	}
	cache := p.newStructuralCache()
	root := Handle{Label: "workspace", Ref: Ref{Kind: KindSpace, ID: WorkspaceRefID}, Level: LevelCard}

	nodes := map[Ref]Handle{root.Ref: root}
	edges := map[GraphEdge]struct{}{}
	queue := []Ref{root.Ref}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		children, err := p.graphChildren(ctx, cache, current)
		if err != nil {
			if optionalSourceMissing(current) {
				continue
			}
			return Graph{}, err
		}
		for _, child := range children {
			edges[GraphEdge{Source: current, Target: child.Ref}] = struct{}{}
			if _, seen := nodes[child.Ref]; seen {
				continue
			}
			nodes[child.Ref] = child
			queue = append(queue, child.Ref)
		}
	}

	out := Graph{Nodes: make([]Handle, 0, len(nodes)), Edges: make([]GraphEdge, 0, len(edges))}
	for _, handle := range nodes {
		out.Nodes = append(out.Nodes, handle)
	}
	sort.Slice(out.Nodes, func(i, j int) bool {
		return out.Nodes[i].Ref.String() < out.Nodes[j].Ref.String()
	})
	for edge := range edges {
		out.Edges = append(out.Edges, edge)
	}
	sort.Slice(out.Edges, func(i, j int) bool {
		a, b := out.Edges[i], out.Edges[j]
		if a.Source != b.Source {
			return a.Source.String() < b.Source.String()
		}
		return a.Target.String() < b.Target.String()
	})
	return out, nil
}

// graphChildren is the uncapped child list the whole-map walk follows. It is
// structuralCache.children plus the two trajectory edges Children knows about
// (session -> its Rota, Rota -> the entities bound under its nodes), so the map
// shows exactly the graph an agent's expand tool walks.
func (p *Projector) graphChildren(ctx context.Context, cache *structuralCache, ref Ref) ([]Handle, error) {
	switch ref.Kind {
	case KindSession:
		workers, err := cache.children(ctx, ref)
		if err != nil {
			return nil, err
		}
		if h := p.trajectoryHandleFor(ctx, ref.ID); h != nil {
			workers = append([]Handle{*h}, workers...)
		}
		return workers, nil
	case KindTrajectory:
		return p.trajectoryChildren(ctx, ref.ID)
	default:
		return cache.children(ctx, ref)
	}
}

// optionalSourceMissing reports whether a failing node is one of the categories
// backed by an optional projector source. Their absence is a configuration
// state, not a broken workspace.
func optionalSourceMissing(ref Ref) bool {
	return ref.Kind == KindCategory && (ref.ID == CategorySkills || ref.ID == CategoryInsights)
}

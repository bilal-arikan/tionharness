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
	// Live is the sessions executing right now (see GraphLive); empty, never
	// null, when nothing runs or no live source is attached.
	Live []GraphLive `json:"live"`
	// Meta carries per-session facet data (kind / agent / tags / archived) keyed
	// by Ref.String, for the map's filters.
	Meta map[string]GraphMeta `json:"meta"`
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
	nodes, edges, err := p.GraphStructure(ctx)
	if err != nil {
		return Graph{}, err
	}
	live, meta, err := p.GraphLive(ctx)
	if err != nil {
		return Graph{}, err
	}
	return Graph{Nodes: nodes, Edges: edges, Live: live, Meta: meta}, nil
}

// GraphLive is the per-request half of Graph: which sessions execute right now
// plus the per-session facet meta. It is cheap (one session pass) and depends
// on the live source, so it is never cached.
func (p *Projector) GraphLive(ctx context.Context) ([]GraphLive, map[string]GraphMeta, error) {
	if p == nil || p.store == nil {
		return nil, nil, fmt.Errorf("view: projector has no store")
	}
	return p.graphLive(ctx, p.newStructuralCache())
}

// GraphStructure is the structural half of Graph: the breadth-first walk of
// every node and edge from the workspace root. It depends only on the store's
// entities (plus the optional static sources), which is what lets the API
// memoise it against db.MutationGen across requests.
func (p *Projector) GraphStructure(ctx context.Context) ([]Handle, []GraphEdge, error) {
	if p == nil || p.store == nil {
		return nil, nil, fmt.Errorf("view: projector has no store")
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
			return nil, nil, err
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

	out := Graph{
		Nodes: make([]Handle, 0, len(nodes)),
		Edges: make([]GraphEdge, 0, len(edges)),
	}
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
	return out.Nodes, out.Edges, nil
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

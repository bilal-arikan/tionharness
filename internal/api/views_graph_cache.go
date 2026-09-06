package api

import (
	"context"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/view"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// graphStructureTTL bounds how long a cached structure may be served without a
// rebuild even when the store generation has not moved. The structural walk
// also reads sources outside the store (skills catalog, built-in tool groups)
// that do not bump db.MutationGen, so a stale entry is bounded to this window.
const graphStructureTTL = 15 * time.Second

// graphStructureEntry is one workspace's memoised Explorer structure.
type graphStructureEntry struct {
	gen     uint64
	builtAt time.Time
	nodes   []view.Handle
	edges   []view.GraphEdge
}

// graphStructureCache is keyed by workspace id. Entries are immutable once
// stored (handlers only read the slices), so a plain sync.Map suffices.
var graphStructureCache sync.Map

// cachedGraphStructure returns the workspace's Explorer node/edge lists,
// rebuilding them only when an entity mutation happened since the last build
// or the entry aged past graphStructureTTL. The whole-workspace breadth-first
// walk was redone on every GET /api/views/graph — every map open, every
// live-layer refresh — while the entities it walks change far less often than
// the map is looked at.
func cachedGraphStructure(ctx context.Context, wsp *workspace.Workspace, p *view.Projector) ([]view.Handle, []view.GraphEdge, error) {
	gen := wsp.DB.MutationGen()
	if v, ok := graphStructureCache.Load(wsp.ID); ok {
		e := v.(*graphStructureEntry)
		if e.gen == gen && time.Since(e.builtAt) < graphStructureTTL {
			return e.nodes, e.edges, nil
		}
	}
	nodes, edges, err := p.GraphStructure(ctx)
	if err != nil {
		return nil, nil, err
	}
	graphStructureCache.Store(wsp.ID, &graphStructureEntry{gen: gen, builtAt: time.Now(), nodes: nodes, edges: edges})
	return nodes, edges, nil
}

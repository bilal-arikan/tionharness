package api

// handleGetView exposes the projection layer (internal/view) over HTTP: the
// compact, deterministic summary of a large piece of runtime state.
//
//	GET /api/views/{kind}/{id}?level=card&sub=<nodeId>
//
// The response carries both the structured envelope and `text` — the exact bytes
// an agent would receive. The panel renders `text` verbatim rather than
// re-composing it from the parts, so what the user sees and what the model sees
// can never drift apart.

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/view"
)

// viewProjector builds the projection resolver for the current request. The
// wiring itself (workspace name + the optional skills / findings / logs sources)
// lives in tools.ViewProjector, the single place every caller — this handler,
// the dashboard, get_view and expand — goes through, so no surface can end up
// with a differently-configured projector than the others.
func (s *Server) viewProjector(r *http.Request) *view.Projector {
	src := tools.ViewSources{Logs: s.logs, DefaultAgentID: ws(r).Settings().DefaultAgentId}
	if rt := ws(r).Runtime; rt != nil {
		src.Skills = rt.Skills()
		// Explorer live layer: which sessions are executing right now.
		src.Running = s.liveSessions(ws(r)).RunningSet()
		// Memory notes: the Notlar category and the note leaves. Without this the
		// map drew an empty bucket and GET /api/views/note/{id} answered "notes
		// store unavailable" while the agent's own get_view listed the notes.
		src.Notes = rt.Notes()
	}
	return tools.ViewProjector(ws(r).DB, ws(r).Name, src)
}

func (s *Server) handleGetView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ref := view.Ref{
		Kind: view.Kind(strings.TrimSpace(r.PathValue("kind"))),
		ID:   strings.TrimSpace(r.PathValue("id")),
		Sub:  strings.TrimSpace(q.Get("sub")),
	}

	v, err := s.viewProjector(r).Project(r.Context(), ref, view.ParseLevel(q.Get("level")))
	if err != nil {
		// An unknown kind is a client mistake; a missing entity is a 404. Both are
		// reported instead of degrading to an empty view, which would read like a
		// healthy but empty entity.
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "unsupported kind") || strings.Contains(err.Error(), "no id") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ref":        v.Ref,
		"level":      v.Level,
		"header":     v.Header,
		"body":       v.Body,
		"text":       v.Text(),
		"handles":    v.Handles,
		"asOf":       v.AsOf,
		"source":     v.Source,
		"elided":     v.Elided,
		"elidedUnit": v.ElidedUnit,
		"tokens":     v.Tokens,
	})
}

// handleGetViewChildren exposes the Explorer map's structural drill-down:
//
//	GET /api/views/{kind}/{id}/children?sub=<selector>
//
// It returns the child handles of one node — what expanding it reveals — without
// rendering a full card for each child. This is the map's lazy-expand edge, the
// same graph an agent walks; the node's own summary (with its elision count) comes
// from GET /api/views/{kind}/{id}.
func (s *Server) handleGetViewChildren(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ref := view.Ref{
		Kind: view.Kind(strings.TrimSpace(r.PathValue("kind"))),
		ID:   strings.TrimSpace(r.PathValue("id")),
		Sub:  strings.TrimSpace(q.Get("sub")),
	}

	handles, err := s.viewProjector(r).Children(r.Context(), ref)
	if err != nil {
		// An unsupported kind is a client mistake (400); anything else is a store
		// read failure. Neither degrades to an empty list, which would read like a
		// genuine leaf node and hide the error.
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "children unsupported") || strings.Contains(err.Error(), "no id") ||
			strings.Contains(err.Error(), "unknown category") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	// Never emit a null JSON array — a node with no children returns [].
	if handles == nil {
		handles = []view.Handle{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ref":      ref,
		"children": handles,
	})
}

// handleGetViewNeighborhood exposes the Explorer focus graph's complete direct
// neighborhood. The projector is built from ws(r), so refs can resolve only
// against the workspace selected by the request middleware.
func (s *Server) handleGetViewNeighborhood(w http.ResponseWriter, r *http.Request) {
	ref := view.Ref{
		Kind: view.Kind(strings.TrimSpace(r.PathValue("kind"))),
		ID:   strings.TrimSpace(r.PathValue("id")),
		Sub:  strings.TrimSpace(r.URL.Query().Get("sub")),
	}

	neighborhood, err := s.viewProjector(r).Neighborhood(r.Context(), ref)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "unsupported kind") || strings.Contains(err.Error(), "children unsupported") ||
			strings.Contains(err.Error(), "no id") || strings.Contains(err.Error(), "unknown category") {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	if neighborhood.Parents == nil {
		neighborhood.Parents = []view.Handle{}
	}
	if neighborhood.Children == nil {
		neighborhood.Children = []view.Handle{}
	}
	writeJSON(w, http.StatusOK, neighborhood)
}

// handleGetViewGraph exposes the whole structural map of the active workspace:
//
//	GET /api/views/graph
//
// Every node reachable from the workspace root plus every parent -> child edge,
// uncapped, so the Explorer screen can lay the entire hierarchy out as one
// force-directed network instead of paging through neighborhoods.
func (s *Server) handleGetViewGraph(w http.ResponseWriter, r *http.Request) {
	p := s.viewProjector(r)
	nodes, edges, err := cachedGraphStructure(r.Context(), ws(r), p)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	live, meta, err := p.GraphLive(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	graph := view.Graph{Nodes: nodes, Edges: edges, Live: live, Meta: meta}
	// Never emit null arrays: an empty workspace still has a root node, and an
	// edge-less graph is [] not null.
	if graph.Nodes == nil {
		graph.Nodes = []view.Handle{}
	}
	if graph.Edges == nil {
		graph.Edges = []view.GraphEdge{}
	}
	if graph.Live == nil {
		graph.Live = []view.GraphLive{}
	}
	if graph.Meta == nil {
		graph.Meta = map[string]view.GraphMeta{}
	}
	writeJSON(w, http.StatusOK, graph)
}

// registerViewRoutes mounts the projection layer and the dashboard that is built
// on it. These used to be registered inside registerFlowRoutes; when the flows
// surface was rebuilt (2026-10-03) that function was replaced wholesale and the
// view + dashboard lines went with it, so GET /api/views/graph fell through to the
// SPA catch-all and the Explorer screen loaded HTML as JSON. They live here, next
// to their handlers, and routes_test.go pins every /api path the frontend calls.
func (s *Server) registerViewRoutes(mux *http.ServeMux) {
	// Whole-workspace structural map for the Explorer network (_Docs/68).
	// Registered before the {kind}/{id} pattern only for readability; "graph" is
	// a literal segment and never collides with a two-segment ref.
	mux.HandleFunc("GET /api/views/graph", s.handleGetViewGraph)
	// One projection: the same bytes the agent gets from get_view (_Docs/66).
	mux.HandleFunc("GET /api/views/{kind}/{id}", s.handleGetView)
	// Explorer map drill-down: the structural child handles of a node.
	mux.HandleFunc("GET /api/views/{kind}/{id}/children", s.handleGetViewChildren)
	// Explorer focus graph: complete direct parents + children, uncapped.
	mux.HandleFunc("GET /api/views/{kind}/{id}/neighborhood", s.handleGetViewNeighborhood)

	// Workspace overview: counters + chart series in one call (_Docs/66).
	mux.HandleFunc("GET /api/dashboard", s.handleDashboard)
	mux.HandleFunc("GET /api/dashboard/commit-activity", s.handleDashboardCommitActivity)
}

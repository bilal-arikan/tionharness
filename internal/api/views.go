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
	src := tools.ViewSources{Logs: s.logs}
	if rt := ws(r).Runtime; rt != nil {
		src.Skills = rt.Skills()
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

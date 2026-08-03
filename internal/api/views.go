package api

// handleGetView exposes the projection layer (internal/view) over HTTP: the
// compact, deterministic summary of a large piece of runtime state.
//
//	GET /api/views/{kind}/{id}?level=card&lens=health&sub=<nodeId>
//
// The response carries both the structured envelope and `text` — the exact bytes
// an agent would receive. The panel renders `text` verbatim rather than
// re-composing it from the parts, so what the user sees and what the model sees
// can never drift apart.

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/view"
)

func (s *Server) handleGetView(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ref := view.Ref{
		Kind: view.Kind(strings.TrimSpace(r.PathValue("kind"))),
		ID:   strings.TrimSpace(r.PathValue("id")),
		Sub:  strings.TrimSpace(q.Get("sub")),
	}

	v, err := view.NewProjector(ws(r).DB).Project(
		r.Context(), ref, view.ParseLevel(q.Get("level")), view.ParseLens(q.Get("lens")))
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
		"lens":       v.Lens,
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

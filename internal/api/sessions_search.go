package api

import (
	"net/http"
	"strconv"

	"github.com/bilal-arikan/swarmgo/internal/db"
)

// handleSearchMessages full-text searches the workspace's message history.
// GET /api/sessions/search?q=...&limit=...&role=user|assistant&exclude=<sessionId>
// Workspace-scoped (ws(r).DB); returns []db.SearchHit so the UI can deep-link to
// the matching session + message.
func (s *Server) handleSearchMessages(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeError(w, http.StatusBadRequest, "q (query) is required")
		return
	}
	opts := db.SearchOpts{Query: q, ExcludeID: r.URL.Query().Get("exclude")}
	if role := r.URL.Query().Get("role"); role != "" && role != "all" {
		opts.Roles = []string{role}
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			opts.Limit = n
		}
	}

	hits, err := ws(r).DB.SearchMessages(r.Context(), opts)
	if writeDBError(w, err, "") {
		return
	}
	if hits == nil {
		hits = []db.SearchHit{}
	}
	writeJSON(w, http.StatusOK, hits)
}

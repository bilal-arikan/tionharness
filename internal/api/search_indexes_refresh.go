package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// searchIndexRefreshRequest is the body of a refresh request.
//
// Rebuild separates the two operations the user can ask for rather than letting
// the server guess: a refresh re-embeds in place and keeps the store, a rebuild
// discards it first. Guessing would mean either silently throwing away a large
// index or silently refusing to fix a corrupt one.
type searchIndexRefreshRequest struct {
	Tool    string `json:"tool"`
	Root    string `json:"root"`
	Rebuild bool   `json:"rebuild"`
}

// handleSearchIndexRefresh starts a refresh or rebuild of one index.
//
// It runs the action the USER named instead of the one indexstate.Decide would
// pick — the button exists precisely for a store whose manifest looks fine but
// whose contents are not, which the automatic path cannot see. The work itself
// goes through Runtime.RequestIndexRun, so the root guards (absolute, not an
// ephemeral worktree, not a home or volume root) and the single-run lock are
// the same ones every other index run obeys.
//
// It answers as soon as the run is CLAIMED: indexing takes minutes, far longer
// than an HTTP request may wait, so the outcome is reported through the ledger
// that GET /api/search-indexes serves, not through this response.
func (s *Server) handleSearchIndexRefresh(w http.ResponseWriter, r *http.Request) {
	var req searchIndexRefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "geçersiz istek gövdesi: "+err.Error())
		return
	}
	req.Tool = strings.TrimSpace(req.Tool)
	req.Root = strings.TrimSpace(req.Root)
	if req.Tool == "" || req.Root == "" {
		writeError(w, http.StatusBadRequest, "tool ve root zorunlu")
		return
	}

	wsp := ws(r)
	if wsp == nil || wsp.Runtime == nil {
		writeError(w, http.StatusConflict, "aktif çalışma alanı yok")
		return
	}

	action := indexstate.ActionRefresh
	if req.Rebuild {
		action = indexstate.ActionRebuild
	}
	s.logger.Info("search index refresh requested", "tool", req.Tool, "root", req.Root, "action", action)

	entry, err := wsp.Runtime.RequestIndexRun(r.Context(), agent.IndexRequest{
		Tool:   req.Tool,
		Root:   req.Root,
		Action: action,
	})
	if err != nil {
		switch {
		// A run already in flight is a conflict, not a failure: the user asked for
		// something that is already happening, so the panel says so and keeps
		// polling rather than reporting an error.
		case errors.Is(err, agent.ErrIndexRunInFlight):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, agent.ErrIndexRootNotAllowed):
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, agent.ErrUnknownIndexTool):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			s.logger.Warn("search index refresh failed", "tool", req.Tool, "root", req.Root, "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// The claimed entry's Action is the authoritative answer, not the requested
	// one: a refresh of an index that does not exist yet resolves to a create,
	// and the panel must show what is actually running.
	writeJSON(w, http.StatusAccepted, map[string]any{
		"tool":    entry.Tool,
		"root":    entry.Root,
		"action":  entry.Action,
		"started": true,
	})
}

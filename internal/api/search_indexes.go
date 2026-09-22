package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// searchIndexStatus is one (tool, root) index's lifecycle state for the UI.
//
// Phase is the authoritative field and is never inferred client-side: a failed
// index reports "failed" with its reason, so the panel cannot show a broken
// store as working.
type searchIndexStatus struct {
	Tool        string `json:"tool"`
	Root        string `json:"root"`
	Phase       string `json:"phase"`
	Action      string `json:"action,omitempty"`
	Embedding   string `json:"embedding,omitempty"`
	ToolVersion string `json:"toolVersion,omitempty"`
	Error       string `json:"error,omitempty"`
	StartedAt   string `json:"startedAt,omitempty"`
	UpdatedAt   string `json:"updatedAt,omitempty"`
	// Usable says whether a search against this index would answer at all —
	// ready or stale, never failed or missing.
	Usable bool `json:"usable"`
}

// handleSearchIndexes lists the lifecycle state of every search index TionHarness
// has touched in this process, keyed by (tool, root).
//
// The ledger is process-wide rather than per-workspace on purpose: two
// workspaces opened on the same repository share one store on disk, so a
// workspace-scoped view would show the same index twice with two different
// answers.
func (s *Server) handleSearchIndexes(w http.ResponseWriter, r *http.Request) {
	entries := agent.IndexLedger().List()
	out := make([]searchIndexStatus, len(entries))
	for i, e := range entries {
		out[i] = searchIndexStatus{
			Tool:        e.Tool,
			Root:        e.Root,
			Phase:       string(e.Phase),
			Action:      e.Action,
			Embedding:   e.Embedding,
			ToolVersion: e.ToolVersion,
			Error:       e.Error,
			Usable:      e.Usable(),
		}
		if !e.StartedAt.IsZero() {
			out[i].StartedAt = e.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		if !e.UpdatedAt.IsZero() {
			out[i].UpdatedAt = e.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z")
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// searchIndexDropRequest is the body of a drop request.
//
// ConfirmRoot is the confirmation gate and must repeat Root exactly. It is a
// path rather than a boolean deliberately: a `confirm: true` flag is something
// any caller can set, while repeating the path means whoever confirmed knew
// which specific index was being destroyed.
type searchIndexDropRequest struct {
	Tool        string `json:"tool"`
	Root        string `json:"root"`
	ConfirmRoot string `json:"confirmRoot"`
}

// handleSearchIndexDrop deletes one index from disk.
//
// This is a USER action. It is reachable only from the Settings panel and is NOT
// exposed as an agent tool, so an agent cannot destroy an index it merely
// decided was stale. A request without a matching confirmation answers 409 and
// deletes nothing.
func (s *Server) handleSearchIndexDrop(w http.ResponseWriter, r *http.Request) {
	var req searchIndexDropRequest
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

	s.logger.Info("search index drop requested", "tool", req.Tool, "root", req.Root)
	if err := wsp.Runtime.DropSearchIndex(req.Tool, req.Root, req.ConfirmRoot); err != nil {
		switch {
		case errors.Is(err, indexstate.ErrDropNotConfirmed), errors.Is(err, indexstate.ErrDropRootMismatch):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, agent.ErrUnknownIndexTool):
			writeError(w, http.StatusNotFound, err.Error())
		default:
			s.logger.Warn("search index drop failed", "tool", req.Tool, "root", req.Root, "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	s.logger.Info("search index dropped", "tool", req.Tool, "root", req.Root)
	writeJSON(w, http.StatusOK, map[string]any{"tool": req.Tool, "root": req.Root, "dropped": true})
}

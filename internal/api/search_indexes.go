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
	if err := wsp.Runtime.DropSearchIndex(r.Context(), req.Tool, req.Root, req.ConfirmRoot); err != nil {
		if code := writeDropError(w, err); code == http.StatusInternalServerError {
			s.logger.Warn("search index drop failed", "tool", req.Tool, "root", req.Root, "error", err)
		}
		return
	}
	s.logger.Info("search index dropped", "tool", req.Tool, "root", req.Root)
	writeJSON(w, http.StatusOK, map[string]any{"tool": req.Tool, "root": req.Root, "dropped": true})
}

// writeDropError maps a drop failure to its HTTP answer and returns the status
// written, so the caller can log only the unexpected ones.
//
// The reason text always travels in the body: the Settings panel prints it
// verbatim, and a confirmation mismatch is something the user has to read to
// act on. A failed confirmation is 409 (retryable by confirming correctly), as
// is a drop refused because an index run is in flight (retryable once it
// finishes), while an unknown tool is 404 — confirming harder cannot fix it.
func writeDropError(w http.ResponseWriter, err error) int {
	var status int
	switch {
	case errors.Is(err, indexstate.ErrDropNotConfirmed), errors.Is(err, indexstate.ErrDropRootMismatch),
		errors.Is(err, agent.ErrIndexRunInFlight):
		status = http.StatusConflict
	case errors.Is(err, agent.ErrUnknownIndexTool):
		status = http.StatusNotFound
	default:
		status = http.StatusInternalServerError
	}
	writeError(w, status, err.Error())
	return status
}

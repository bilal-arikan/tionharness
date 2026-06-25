package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/progress"
)

// progressView is the client view of a session's persistent progress file: the
// resolved file path, whether it exists, and the decoded record (todos + log).
type progressView struct {
	Path   string           `json:"path"`   // absolute path of the progress file
	Exists bool             `json:"exists"` // whether the file is present
	Record *progress.Record `json:"record"` // decoded record (nil when absent)
}

// handleSessionProgress returns the session's persistent progress (the durable
// todo_write checklist + rolling log) for the read-only viewer card. Resolves the
// same directory the todo sink writes to: the session's effective working dir,
// else the per-agent store fallback.
func (s *Server) handleSessionProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	wsp := ws(r)
	session, err := wsp.DB.GetSession(ctx, id)
	if writeDBError(w, err, "session not found") {
		return
	}
	cwd := strings.TrimSpace(session.WorkingDir)
	if cwd == "" {
		cwd = wsp.Runtime.WorkspaceDefaultDir()
	}
	dir := progressDir(wsp.DB, cwd, session.AgentID)
	view := progressView{Path: progress.File(dir)}
	if rec, ok, loadErr := progress.Load(dir); loadErr == nil && ok {
		view.Exists = true
		view.Record = &rec
	}
	writeJSON(w, http.StatusOK, view)
}

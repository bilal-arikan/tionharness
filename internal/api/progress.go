package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/progress"
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
// SAME directory the todo sink writes to via Runtime.ProgressDir: the session's
// explicit project working dir (shared across sessions on that project), else a
// per-session fallback (so unrelated sessions don't share one progress file).
func (s *Server) handleSessionProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	wsp := ws(r)
	if _, err := wsp.DB.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	dir := wsp.Runtime.ProgressDir(id)
	view := progressView{Path: progress.File(dir)}
	if rec, ok, loadErr := progress.Load(dir); loadErr == nil && ok {
		view.Exists = true
		view.Record = &rec
	}
	writeJSON(w, http.StatusOK, view)
}

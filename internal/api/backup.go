package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/backup"
)

// backupTargets maps the workspace manager's targets into the backup package's
// Target shape (used by the archive-listing endpoint).
func (s *Server) backupTargets() []backup.Target {
	in := s.workspaces.BackupTargets()
	out := make([]backup.Target, 0, len(in))
	for _, t := range in {
		out = append(out, backup.Target{ID: t.ID, Name: t.Name, Dir: t.Dir})
	}
	return out
}

// handleBackupStatus reports the live backup configuration and last-run info.
func (s *Server) handleBackupStatus(w http.ResponseWriter, _ *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup manager not available")
		return
	}
	writeJSON(w, http.StatusOK, s.backups.Status())
}

// handleBackupRun triggers an immediate backup pass over every workspace and
// returns the result. Works regardless of whether the periodic schedule is on,
// so the user can snapshot on demand.
func (s *Server) handleBackupRun(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup manager not available")
		return
	}
	res, err := s.backups.RunOnce(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleListArchives returns the on-disk archives grouped per workspace
// (newest first), so the UI can list and restore them.
func (s *Server) handleListArchives(w http.ResponseWriter, _ *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup manager not available")
		return
	}
	writeJSON(w, http.StatusOK, s.backups.ListArchives(s.backupTargets()))
}

type restoreBackupReq struct {
	WorkspaceID string `json:"workspaceId"`
	Archive     string `json:"archive"`
}

// handleRestoreBackup replaces a workspace's content with a backup archive and
// reopens it live. Destructive: the workspace's current data is overwritten.
func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup manager not available")
		return
	}
	req, ok := bindJSON[restoreBackupReq](w, r)
	if !ok {
		return
	}
	archivePath, err := s.backups.ResolveArchive(req.WorkspaceID, req.Archive)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.workspaces.RestoreFromArchive(req.WorkspaceID, archivePath, backup.Unzip); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The workspace was reopened from disk → its agents/sessions/tasks changed;
	// nudge open UIs to reload their lists.
	s.publishWorkspacesChanged("Workspace bir yedekten geri yüklendi: " + req.WorkspaceID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "workspaceId": req.WorkspaceID, "archive": req.Archive})
}

// handleDeleteArchive removes a single backup archive file.
func (s *Server) handleDeleteArchive(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "backup manager not available")
		return
	}
	// same shape as restore: {workspaceId, archive}
	req, ok := bindJSON[restoreBackupReq](w, r)
	if !ok {
		return
	}
	if err := s.backups.DeleteArchive(req.WorkspaceID, req.Archive); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "workspaceId": req.WorkspaceID, "archive": req.Archive})
}

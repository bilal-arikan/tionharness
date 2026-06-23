package api

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/bilal-arikan/swarmgo/internal/config"
)

// handleLogsPath returns the absolute on-disk log file path the Logs screen can
// copy (the same file SetupLogging mirrors stdout into).
func (s *Server) handleLogsPath(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"path": config.LogFilePath()})
}

// handleRevealLogs opens the folder holding the log file in the OS file manager
// (Windows: Explorer, highlighting the file). If the file does not exist yet
// (logging fell back to stdout-only), the containing folder is opened instead.
func (s *Server) handleRevealLogs(w http.ResponseWriter, r *http.Request) {
	path := config.LogFilePath()
	target := "/select," + path
	if _, err := os.Stat(path); err != nil {
		path = filepath.Dir(path)
		target = path
	}
	// Detached from r.Context(): a fire-and-forget launch must not be killed when
	// the handler returns. explorer.exe returns non-zero even on success.
	if err := exec.Command("explorer.exe", target).Start(); err != nil {
		s.logger.Warn("reveal logs folder failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}

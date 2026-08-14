package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/config"
)

// handleLogsPath returns the absolute on-disk log file path the Logs screen can
// copy (the same file SetupLogging mirrors stdout into).
func (s *Server) handleLogsPath(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"path": config.LogFilePath()})
}

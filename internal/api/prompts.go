package api

import (
	"net/http"
	"os/exec"

	"github.com/bilal-arikan/swarmgo/internal/agent"
)

type promptsResp struct {
	Dir     string             `json:"dir"`
	Prompts []agent.PromptInfo `json:"prompts"`
}

// handleListPrompts returns the built-in runtime prompts (summaries, reflection,
// titling) plus the source folder that holds them, for read-only display.
func (s *Server) handleListPrompts(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, promptsResp{Dir: agent.PromptsDir(), Prompts: agent.Prompts()})
}

// handleRevealPrompts opens the prompt source folder in the OS file manager on
// the machine running the backend (local desktop app). Windows: Explorer.
func (s *Server) handleRevealPrompts(w http.ResponseWriter, r *http.Request) {
	dir := agent.PromptsDir()
	if dir == "" {
		writeError(w, http.StatusNotFound, "prompt directory unknown")
		return
	}
	// Detached from r.Context() so it isn't killed when the handler returns.
	if err := exec.Command("explorer.exe", dir).Start(); err != nil {
		// explorer.exe returns a non-zero exit code even on success; only a
		// failure to *start* the process is a real error.
		s.logger.Warn("reveal prompts folder failed", "error", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": dir})
}

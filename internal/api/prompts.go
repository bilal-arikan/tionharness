package api

import (
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/agent"
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

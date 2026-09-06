package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// builtinPromptTO carries the compiled-in system prompt of the built-in
// definition an agent's system role resolves to. The System agents screen uses
// it to offer "revert to the code prompt" on the soul editor: a customisation
// that has drifted from the shipped text can be put back without deleting the
// row (which would also drop model/tool choices).
type builtinPromptTO struct {
	// SystemKey is the built-in role the prompt belongs to.
	SystemKey string `json:"systemKey"`
	// Soul is the compiled-in prompt text (prompts.Default for the role).
	Soul string `json:"soul"`
}

// handleAgentBuiltinPrompt returns the compiled-in prompt for the agent's
// system role. 404 when the agent is unknown or carries no system role — a
// plain user agent has no code prompt to revert to.
func (s *Server) handleAgentBuiltinPrompt(w http.ResponseWriter, r *http.Request) {
	current, err := ws(r).DB.GetAgent(r.Context(), r.PathValue("id"))
	if writeDBError(w, err, "agent not found") {
		return
	}
	if current.SystemKey == "" {
		writeError(w, http.StatusNotFound, "agent has no system role")
		return
	}
	def, ok := agent.SystemAgentDefault(current.SystemKey)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown system role: "+current.SystemKey)
		return
	}
	writeJSON(w, http.StatusOK, builtinPromptTO{SystemKey: current.SystemKey, Soul: def.SystemPrompt})
}

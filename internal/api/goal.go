package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/agent"
)

// maxGoalLen bounds a session goal so it can never blow up the context window;
// the UI mirrors this. Goals are meant to be a short objective, not a document.
const maxGoalLen = 2000

// goalContextBlock is the renderer for a session's persistent objective. It now
// lives in the agent package (agent.GoalContextBlock) so the chat and autonomous
// prompt paths share ONE source; this thin alias keeps the api call sites stable.
func goalContextBlock(goal string, done bool) string {
	return agent.GoalContextBlock(goal, done)
}

type setGoalReq struct {
	Goal string `json:"goal"`
	// Done marks the goal as achieved: it stays visible (so the user can review
	// or reopen it) but stops being injected into context. Editing the goal text
	// reopens it (the client sends done=false on a text edit).
	Done bool `json:"done"`
}

// handleSetSessionGoal sets (or clears, when empty) a session's persistent goal
// and its done state.
func (s *Server) handleSetSessionGoal(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setGoalReq](w, r)
	if !ok {
		return
	}
	goal := strings.TrimSpace(req.Goal)
	if len([]rune(goal)) > maxGoalLen {
		writeError(w, http.StatusBadRequest, "goal too long")
		return
	}
	// A cleared goal can't be "done".
	done := req.Done && goal != ""

	ctx := r.Context()
	database := ws(r).DB
	if _, err := database.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := database.SetSessionGoal(ctx, id, goal, done); writeDBError(w, err, "") {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "goal": goal, "goalDone": done})
}

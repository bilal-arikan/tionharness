package api

import (
	"net/http"
	"strings"
)

// maxGoalLen bounds a session goal so it can never blow up the context window;
// the UI mirrors this. Goals are meant to be a short objective, not a document.
const maxGoalLen = 2000

// goalContextBlock renders a session's persistent objective as a system-prompt
// section. Inspired by Claude Code's /goal: a single durable "north star" the
// agent should keep steering toward across turns. Kept in the dynamic (uncached)
// suffix and placed first so it leads the volatile context. Returns "" when no
// goal is set OR when the goal is marked done (a completed objective stops
// steering future turns — the /goal checker convergence: once achieved, drop it).
func goalContextBlock(goal string, done bool) string {
	goal = strings.TrimSpace(goal)
	if goal == "" || done {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Session goal (north star)\n")
	b.WriteString("Keep every reply aligned with this persistent objective and make steady progress toward it; flag when it is achieved or blocked. It persists across turns.\n\n")
	b.WriteString(goal)
	return strings.TrimSpace(b.String())
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
	var req setGoalReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
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

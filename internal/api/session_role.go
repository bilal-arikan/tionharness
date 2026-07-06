package api

import (
	"net/http"
	"strings"
)

type setRoleReq struct {
	Role string `json:"role"`
}

// handleSetSessionRole sets a session's coordinator/worker role (M2, _Docs/47).
// Accepts "coordinator" (enable coordinator mode: the coordinator prompt + the
// spawn_worker/send_to_worker/stop_worker/list_workers tools) or "" (revert to an
// ordinary session). "worker" is rejected here — worker sessions are created by
// spawn_worker, not toggled by hand.
func (s *Server) handleSetSessionRole(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req, ok := bindJSON[setRoleReq](w, r)
	if !ok {
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != "" && role != "coordinator" {
		writeError(w, http.StatusBadRequest, "role must be \"coordinator\" or empty")
		return
	}
	ctx := r.Context()
	database := ws(r).DB
	if _, err := database.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	if err := database.SetSessionRole(ctx, id, role); writeDBError(w, err, "") {
		return
	}
	// Cross-window sync: a sibling window's sidebar swaps the role chip and the
	// coordination tools panel becomes visible/hidden as appropriate.
	emitSessionChange(ws(r), id, "role")
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "role": role})
}

// handleListWorkers returns the workers spawned under a coordinator session, for
// the coordination UI panel.
func (s *Server) handleListWorkers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	wsp := ws(r)
	ctx := r.Context()
	if _, err := wsp.DB.GetSession(ctx, id); writeDBError(w, err, "session not found") {
		return
	}
	workers, err := wsp.Runtime.ListWorkers(ctx, id)
	if writeDBError(w, err, "") {
		return
	}
	out := make([]map[string]any, 0, len(workers))
	for _, wk := range workers {
		out = append(out, map[string]any{
			"sessionId": wk.SessionID,
			"agentName": wk.AgentName,
			"title":     wk.Title,
			"running":   wk.Running,
			"summary":   wk.Summary,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"workers": out})
}

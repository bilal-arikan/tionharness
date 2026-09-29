package api

import "net/http"

// executionRuntimeItem is the live subset of an executionItem: what the chat
// sidebar needs to draw a running dot, a last-run chip and the coordinator
// lineage of a worker. No title, agent name, counters or timestamps.
type executionRuntimeItem struct {
	SessionID                string `json:"sessionId"`
	Running                  bool   `json:"running"`
	LastStatus               string `json:"lastStatus,omitempty"`
	CoordinatorSessionID     string `json:"coordinatorSessionId,omitempty"`
	RootCoordinatorSessionID string `json:"rootCoordinatorSessionId,omitempty"`
}

// handleListExecutionRuntime is GET /api/executions/runtime — the compact twin
// of the executions feed for the polling sidebar. The full feed carries every
// session in the workspace with titles and agent names, and the sidebar
// re-fetched it on every run-lifecycle event only to build a sessionId →
// {running,lastStatus,lineage} map. This returns exactly that map's rows and
// only for sessions that carry any of the three facts (an absent row is idle),
// so the payload stays proportional to the workspace's activity, not its age.
func (s *Server) handleListExecutionRuntime(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	running := s.liveSessions(wsp).RunningSet()
	rows := wsp.DB.RuntimeSessions()
	out := make([]executionRuntimeItem, 0, len(rows)+len(running))
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		item := executionRuntimeItem{
			SessionID:                row.SessionID,
			Running:                  running[row.SessionID],
			LastStatus:               row.LastStatus,
			CoordinatorSessionID:     row.CoordinatorSessionID,
			RootCoordinatorSessionID: row.RootCoordinatorSessionID,
		}
		out = append(out, item)
		seen[row.SessionID] = true
	}
	// Idle chat sessions need no cached row. Add only the ones currently active,
	// including runs that started without mutating the persistent store.
	for id := range running {
		if seen[id] {
			continue
		}
		sess, err := wsp.DB.GetSession(ctx, id)
		if err != nil {
			continue
		}
		out = append(out, executionRuntimeItem{SessionID: id, Running: true, CoordinatorSessionID: sess.CoordinatorSessionID, RootCoordinatorSessionID: sess.RootCoordinator()})
	}
	writeJSON(w, http.StatusOK, out)
}

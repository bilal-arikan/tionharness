package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/db"
)

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
	sessions, err := wsp.DB.ListSessions(ctx, "")
	if writeDBError(w, err, "") {
		return
	}
	running := s.liveSessions(wsp).RunningSet()

	// Flow status index: one store scan, only when a flow session exists.
	var flowStatus, runStatus map[string]string
	for _, sess := range sessions {
		if sess.Kind != "flow" {
			continue
		}
		runs, err := wsp.DB.ListFlowRuns(ctx, "")
		if writeDBError(w, err, "") {
			return
		}
		flowStatus = newestFlowRunStatus(runs)
		runStatus = flowRunStatusByID(runs)
		break
	}

	out := make([]executionRuntimeItem, 0, len(running)*2)
	for _, sess := range sessions {
		item := executionRuntimeItem{
			SessionID:                sess.ID,
			Running:                  running[sess.ID],
			LastStatus:               s.lastStatusFor(ctx, wsp, sess, flowStatus, runStatus),
			CoordinatorSessionID:     sess.CoordinatorSessionID,
			RootCoordinatorSessionID: sess.RootCoordinator(),
		}
		if !item.Running && item.LastStatus == "" && item.CoordinatorSessionID == "" {
			continue
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, out)
}

// executionRuntimeRows is the test seam for the row shape above.
var _ = db.Session{}

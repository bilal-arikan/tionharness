package api

import (
	"context"
	"net/http"
	"sort"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// executionItem is one row of the unified executions feed: a session-backed
// transcript produced by any path (chat, task, flow, schedule),
// enriched with the agent name, a live-running flag and a last-run status so the
// feed renders every execution uniformly.
type executionItem struct {
	SessionID                string `json:"sessionId"`
	Kind                     string `json:"kind"`
	SourceID                 string `json:"sourceId,omitempty"`
	Title                    string `json:"title"`
	AgentID                  string `json:"agentId"`
	AgentName                string `json:"agentName"`
	MessageCount             int    `json:"messageCount"`
	Unread                   bool   `json:"unread"`
	Running                  bool   `json:"running"`
	LastStatus               string `json:"lastStatus,omitempty"`
	CoordinatorSessionID     string `json:"coordinatorSessionId,omitempty"`
	RootCoordinatorSessionID string `json:"rootCoordinatorSessionId,omitempty"`
	CreatedAt                int64  `json:"createdAt"`
	UpdatedAt                int64  `json:"updatedAt"`
}

// handleListExecutions returns the unified executions feed across all agents.
// Optional ?kind= filters to a single category (chat|task|flow|schedule|...).
// Newest-updated first. Because every run path now funnels its output into a
// Session, this single list surfaces them all.
func (s *Server) handleListExecutions(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	kindFilter := r.URL.Query().Get("kind")

	sessions, err := wsp.DB.ListSessions(ctx, "")
	if writeDBError(w, err, "") {
		return
	}

	// Every session working right now (chat stream, autonomous invoke, or any turn
	// holding the admission slot — slash commands included).
	running := s.liveSessions(wsp).RunningSet()

	// Cache agent names so a large feed doesn't re-fetch the same agent.
	names := map[string]string{}
	agentName := func(id string) string {
		if id == "" {
			return ""
		}
		if n, ok := names[id]; ok {
			return n
		}
		n := ""
		if a, err := wsp.DB.GetAgent(ctx, id); err == nil {
			n = a.Name
		}
		names[id] = n
		return n
	}

	// Flow status index: built lazily with a single store scan, and only when a
	// flow-kind session is actually in the feed — a workspace with no flows pays
	// nothing.
	var flowStatus map[string]string
	var runStatus map[string]string
	for _, sess := range sessions {
		if sess.Kind != "flow" || (kindFilter != "" && kindFilter != "flow") {
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

	out := make([]executionItem, 0, len(sessions))
	for _, sess := range sessions {
		if kindFilter != "" && sess.Kind != kindFilter {
			continue
		}
		out = append(out, executionItem{
			SessionID:                sess.ID,
			Kind:                     sess.Kind,
			SourceID:                 sess.SourceID,
			Title:                    sess.Title,
			AgentID:                  sess.OwnerAgentID(),
			AgentName:                sessionAgentName(sess, agentName),
			MessageCount:             sess.MessageCount,
			Unread:                   sess.Unread,
			Running:                  running[sess.ID],
			LastStatus:               s.lastStatusFor(ctx, wsp, sess, flowStatus, runStatus),
			CoordinatorSessionID:     sess.CoordinatorSessionID,
			RootCoordinatorSessionID: sess.RootCoordinator(),
			CreatedAt:                sess.CreatedAt,
			UpdatedAt:                sess.UpdatedAt,
		})
	}
	// Newest-updated first, with a SessionID tie-break so equal-UpdatedAt rows keep
	// a STABLE order across polls (otherwise the feed reshuffles every few seconds).
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].SessionID > out[j].SessionID
	})

	// Paging/sorting contract: when any of limit/offset/sort is present the reply
	// becomes the standard {items,total,offset,limit,hasMore} envelope (same keys
	// as the list_* tools); with none of them it stays the legacy full list, so
	// the polling sidebar keeps its current wire format.
	limit, offset, field, asc, listing, err := listQueryParams(r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !listing {
		writeJSON(w, http.StatusOK, out)
		return
	}
	less, err := tools.SortByField(out, field, asc,
		func(e executionItem) int64 { return e.UpdatedAt },
		func(e executionItem) int64 { return e.CreatedAt },
		func(e executionItem) string { return e.Title },
		func(e executionItem) string { return e.SessionID },
	)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sort.SliceStable(out, less)
	page, total := tools.SlicePage(out, offset, limit)
	pageJSONResponse(w, page, total, offset, limit)
}

// lastStatusFor resolves a session's most recent run status where one exists
// (task → its last run status; flow → its newest run's status). Empty for chat
// and schedule kinds, which have no discrete pass/fail outcome.
//
// flowStatus is the prebuilt flowID → newest-run-status index (see
// newestFlowRunStatus); a nil map simply yields no flow status. It is a parameter
// rather than a lookup because this is called once per session on a poll that
// runs every few seconds: resolving the flow status per session meant a full
// flowRuns scan plus a sort EACH TIME, i.e. O(sessions × flowRuns).
func (s *Server) lastStatusFor(ctx context.Context, wsp *workspace.Workspace, sess db.Session, flowStatus, runStatus map[string]string) string {
	if sess.SourceID == "" {
		return ""
	}
	switch sess.Kind {
	case "task":
		if t, err := wsp.DB.GetTask(ctx, sess.SourceID); err == nil {
			return t.LastRunStatus
		}
	case "flow":
		// A flow session is ONE run's transcript: answer with that run's status
		// (Origin.RunID, stamped at run creation since R1). Only a session that
		// predates the link falls back to the flow's newest run — the old
		// behaviour, which showed a newer run's chip on an older transcript.
		if rid := sess.Lineage().RunID; rid != "" {
			if st, ok := runStatus[rid]; ok {
				return st
			}
		}
		return flowStatus[sess.SourceID]
	}
	return ""
}

// flowRunStatusByID indexes runID → status in one pass.
func flowRunStatusByID(runs []db.FlowRun) map[string]string {
	out := make(map[string]string, len(runs))
	for _, r := range runs {
		out[r.ID] = r.Status
	}
	return out
}

// newestFlowRunStatus indexes flowID → the status of that flow's newest run, in
// ONE scan of the store. runs must be ordered newest-first (what ListFlowRuns
// returns), so the first entry seen for a flow wins.
func newestFlowRunStatus(runs []db.FlowRun) map[string]string {
	out := make(map[string]string, len(runs))
	for _, r := range runs {
		if _, seen := out[r.FlowID]; !seen {
			out[r.FlowID] = r.Status
		}
	}
	return out
}

// sessionAgentName is the feed's display name for a session's agent. A delegated
// run names its agent as a target rather than an owner, and a profile subagent
// has no agent row at all — without both fallbacks those rows read as unowned.
func sessionAgentName(sess db.Session, lookup func(string) string) string {
	if n := lookup(sess.OwnerAgentID()); n != "" {
		return n
	}
	return sess.OwnerProfileLabel()
}

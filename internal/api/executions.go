package api

import (
	"context"
	"net/http"
	"sort"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/tools"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// executionItem is one row of the unified executions feed: a session-backed
// transcript produced by any path (chat, task, flow, schedule),
// enriched with the agent name, a live-running flag and a last-run status so the
// feed renders every execution uniformly.
type executionItem struct {
	SessionID    string `json:"sessionId"`
	Kind         string `json:"kind"`
	SourceID     string `json:"sourceId,omitempty"`
	Title        string `json:"title"`
	AgentID      string `json:"agentId"`
	AgentName    string `json:"agentName"`
	MessageCount int    `json:"messageCount"`
	Unread       bool   `json:"unread"`
	Running      bool   `json:"running"`
	LastStatus   string `json:"lastStatus,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
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

	// Merge chat streaming sessions and autonomous (schedule) sessions.
	running := map[string]bool{}
	for _, id := range s.runs.activeSessionIDs(wsp.ID) {
		running[id] = true
	}
	for _, id := range wsp.Runtime.ActiveSessionIDs() {
		running[id] = true
	}

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

	out := make([]executionItem, 0, len(sessions))
	for _, sess := range sessions {
		if kindFilter != "" && sess.Kind != kindFilter {
			continue
		}
		out = append(out, executionItem{
			SessionID:    sess.ID,
			Kind:         sess.Kind,
			SourceID:     sess.SourceID,
			Title:        sess.Title,
			AgentID:      sess.AgentID,
			AgentName:    agentName(sess.AgentID),
			MessageCount: sess.MessageCount,
			Unread:       sess.Unread,
			Running:      running[sess.ID],
			LastStatus:   s.lastStatusFor(ctx, wsp, sess),
			CreatedAt:    sess.CreatedAt,
			UpdatedAt:    sess.UpdatedAt,
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
func (s *Server) lastStatusFor(ctx context.Context, wsp *workspace.Workspace, sess db.Session) string {
	switch sess.Kind {
	case "task":
		if sess.SourceID == "" {
			return ""
		}
		if t, err := wsp.DB.GetTask(ctx, sess.SourceID); err == nil {
			return t.LastRunStatus
		}
	case "flow":
		if sess.SourceID == "" {
			return ""
		}
		if runs, err := wsp.DB.ListFlowRuns(ctx, sess.SourceID); err == nil && len(runs) > 0 {
			return runs[0].Status
		}
	}
	return ""
}

package api

import (
	"context"
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// activityState reports which nav views currently have work in progress, so the
// left rail can show a live "busy" indicator on them. It unifies the
// authoritative running signals for the active workspace:
//   - chat:       an in-flight streaming chat turn (any window)
//   - task:       a task run in the running state (manual / dependency / schedule)
//   - flow:       a flow run in the running state
//   - schedule:   a running run that was triggered by a schedule
//   - executions: ANY in-flight run — chat stream, autonomous invoke (schedule
//     wake, spawned agent session, inbox delivery) or a running task/flow. This
//     is what lights the unified "Aktivite" view, which would otherwise stay
//     dark for background sessions an agent/flow/schedule spins up on its own.
//
// Because every run path persists its running state (or registers an active
// session), this covers both interactive (streamed) and autonomous executions.
type activityState struct {
	Chat       bool `json:"chat"`
	Task       bool `json:"task"`
	Flow       bool `json:"flow"`
	Schedule   bool `json:"schedule"`
	Executions bool `json:"executions"`
}

// handleActivity computes the per-view busy flags for the active workspace.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	var st activityState

	// Every session with a turn in flight: streamed chat turns AND autonomous
	// runtime invokes (schedule wake, spawned agent sessions, inbox delivery,
	// flow agent nodes). Deduped so a session counted by both registries lights
	// the view once. ANY entry lights the unified executions ("Aktivite") view.
	// s.runs is a SERVER-WIDE registry (shared across all workspaces), so both
	// sources are scoped to THIS workspace — otherwise an in-flight turn in the
	// workspace we just left would light an idle workspace's "busy" indicators.
	active := map[string]struct{}{}
	for _, sid := range s.runs.activeSessionIDs(wsp.ID) {
		active[sid] = struct{}{}
	}
	for _, sid := range wsp.Runtime.ActiveSessionIDs() {
		active[sid] = struct{}{}
	}
	for sid := range active {
		sess, err := wsp.DB.GetSession(ctx, sid)
		if err != nil {
			continue
		}
		st.Executions = true
		// Map the session kind to its dedicated view when one exists, so e.g. a
		// flow's background session also lights the Akışlar item, not just Aktivite.
		switch sess.Kind {
		case "", "chat":
			st.Chat = true
		case "flow":
			st.Flow = true
		case "schedule":
			st.Schedule = true
		}
	}

	// Task / schedule: a running run means a task is executing on the board; a
	// schedule-triggered run also lights the schedules view.
	if runs, err := wsp.DB.ListRunningRuns(ctx); err == nil {
		for _, run := range runs {
			st.Task = true
			st.Executions = true
			if run.Trigger == "schedule" {
				st.Schedule = true
			}
		}
	}

	// Flow: any running flow run.
	if fr, err := wsp.DB.ListRunningFlowRuns(ctx); err == nil && len(fr) > 0 {
		st.Flow = true
		st.Executions = true
	}

	writeJSON(w, http.StatusOK, st)
}

// workspaceRunning reports whether the given workspace has ANY run in flight —
// a streamed chat turn, an autonomous runtime invoke (schedule wake / spawned
// session / inbox delivery / flow agent node), a running task run, or a running
// flow run. It is the cross-workspace-safe subset of handleActivity: it skips the
// per-view breakdown (no session-kind lookup) so it stays cheap to call for every
// workspace on each poll.
func (s *Server) workspaceRunning(ctx context.Context, wsp *workspace.Workspace) bool {
	// A turn in flight: server-wide chat runs scoped to this workspace, plus the
	// workspace runtime's own active (autonomous) sessions.
	if len(s.runs.activeSessionIDs(wsp.ID)) > 0 {
		return true
	}
	if len(wsp.Runtime.ActiveSessionIDs()) > 0 {
		return true
	}
	// A task run executing on the board.
	if runs, err := wsp.DB.ListRunningRuns(ctx); err == nil && len(runs) > 0 {
		return true
	}
	// A flow run in progress.
	if fr, err := wsp.DB.ListRunningFlowRuns(ctx); err == nil && len(fr) > 0 {
		return true
	}
	return false
}

// workspaceActivityItem is one row of the cross-workspace activity report: a
// workspace id and whether it currently has live work. Completed-run signalling
// is NOT carried here — that stays on the SSE unread-badge channel, which already
// marks any non-active workspace that produced activity.
type workspaceActivityItem struct {
	ID      string `json:"id"`
	Running bool   `json:"running"`
}

// handleWorkspacesActivity reports the live-run flag for EVERY workspace, so the
// workspace switcher can pulse the ones with work in progress — not just the
// active workspace (which /api/activity already covers per nav view). Global
// route: it reads no request-bound workspace, so it works with none active.
func (s *Server) handleWorkspacesActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	metas := s.workspaces.List()
	out := make([]workspaceActivityItem, 0, len(metas))
	for _, m := range metas {
		wsp, err := s.workspaces.Get(m.ID)
		if err != nil {
			continue
		}
		out = append(out, workspaceActivityItem{ID: m.ID, Running: s.workspaceRunning(ctx, wsp)})
	}
	writeJSON(w, http.StatusOK, out)
}

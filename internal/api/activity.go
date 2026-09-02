package api

import (
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// activityState reports which nav views currently have work in progress, so the
// left rail can show a live "busy" indicator on them. It unifies the
// authoritative running signals for the active workspace:
//   - chat:       an in-flight streaming chat turn (any window)
//   - flow:       a flow run in the running state, or a flow session mid-turn
//   - schedule:   a schedule session mid-turn
//   - executions: ANY in-flight run — chat stream, autonomous invoke (schedule
//     wake, spawned agent session, inbox delivery) or a running flow. This
//     is what lights the unified "Aktivite" view, which would otherwise stay
//     dark for background sessions an agent/flow/schedule spins up on its own.
//
// There is deliberately NO board/task flag: the board does not execute tasks, so
// nothing could ever set it. It used to be derived from the Run entity, which was
// removed once it turned out nothing had written a Run in a long time — the flag
// was reporting "idle" not because the board was idle but because the signal was
// dead. A permanently-false field in this reply is worse than no field: it reads
// as a working indicator.
//
// Because every run path persists its running state (or registers an active
// session), this covers both interactive (streamed) and autonomous executions.
type activityState struct {
	Chat       bool `json:"chat"`
	Flow       bool `json:"flow"`
	Schedule   bool `json:"schedule"`
	Executions bool `json:"executions"`
	Insights   bool `json:"insights"` // a retrospective insight scan is running
}

// handleActivity computes the per-view busy flags for the active workspace.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	var st activityState

	// Every session working right now — see runningSessionIDs for the sources and
	// why each is scoped. ANY entry lights the unified executions ("Aktivite") view.
	active := s.liveSessions(wsp).RunningSet()
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

	// Flow: any running flow run. Pure existence question, so the counter IS the
	// answer — no scan, and no store lock, on either path.
	if wsp.DB.HasRunningFlowRuns() {
		st.Flow = true
		st.Executions = true
	}

	// Insight: a retrospective scan in flight lights the İçgörü nav item.
	st.Insights = wsp.Runtime.InsightScanActive()

	writeJSON(w, http.StatusOK, st)
}

// workspaceRunning reports whether the given workspace has ANY run in flight —
// a streamed chat turn, an autonomous runtime invoke (schedule wake / spawned
// session / inbox delivery / flow agent node), or a running flow run. It is the cross-workspace-safe subset of handleActivity: it skips the
// per-view breakdown (no session-kind lookup) so it stays cheap to call for every
// workspace on each poll.
//
// Every source is O(1) and allocation-free: a lock-free atomic counter plus
// early-exit registry probes. It used to scan the runs and flowRuns maps in
// full — under d.mu, the same lock appendMessageLocked holds across a synchronous
// file write — once per workspace per poll, which with ten workspaces and five
// windows was ~25 whole-store scans per second while completely idle.
func (s *Server) workspaceRunning(wsp *workspace.Workspace) bool {
	return s.runs.hasActive(wsp.ID) || // streamed chat turn in this workspace
		wsp.Runtime.HasActiveSessions() || // autonomous invoke
		wsp.Runtime.HasBusyTurns() || // any turn holding a session slot (slash commands included)
		wsp.DB.HasRunningFlowRuns() || // flow run in progress
		wsp.Runtime.InsightScanActive() // retrospective scan
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
	metas := s.workspaces.List()
	out := make([]workspaceActivityItem, 0, len(metas))
	for _, m := range metas {
		wsp, err := s.workspaces.Get(m.ID)
		if err != nil {
			continue
		}
		out = append(out, workspaceActivityItem{ID: m.ID, Running: s.workspaceRunning(wsp)})
	}
	writeJSON(w, http.StatusOK, out)
}

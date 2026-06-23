package api

import "net/http"

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
	active := map[string]struct{}{}
	for _, sid := range s.runs.activeSessionIDs() {
		active[sid] = struct{}{}
	}
	for _, sid := range wsp.Runtime.ActiveSessionIDs() {
		active[sid] = struct{}{}
	}
	for sid := range active {
		st.Executions = true
		// Map the session kind to its dedicated view when one exists, so e.g. a
		// flow's background session also lights the Akışlar item, not just Aktivite.
		sess, err := wsp.DB.GetSession(ctx, sid)
		if err != nil {
			continue
		}
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

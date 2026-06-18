package api

import "net/http"

// activityState reports which nav views currently have work in progress, so the
// left rail can show a live "busy" indicator on them. It unifies the
// authoritative running signals for the active workspace:
//   - chat:     an in-flight streaming chat turn (any window)
//   - task:     a task run in the running state (manual / dependency / schedule)
//   - flow:     a flow run in the running state
//   - schedule: a running run that was triggered by a schedule
//
// Because every run path persists its running state, this covers both
// interactive (streamed) and autonomous (heartbeat/cron) executions.
type activityState struct {
	Chat     bool `json:"chat"`
	Task     bool `json:"task"`
	Flow     bool `json:"flow"`
	Schedule bool `json:"schedule"`
}

// handleActivity computes the per-view busy flags for the active workspace.
func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	wsp := ws(r)
	ctx := r.Context()
	var st activityState

	// Chat: any in-flight streaming turn on a chat-kind session. activeSessionIDs
	// also includes streamed task/flow runs, so filter by session kind.
	for _, sid := range s.runs.activeSessionIDs() {
		sess, err := wsp.DB.GetSession(ctx, sid)
		if err != nil {
			continue
		}
		if sess.Kind == "" || sess.Kind == "chat" {
			st.Chat = true
			break
		}
	}

	// Task / schedule: a running run means a task is executing on the board; a
	// schedule-triggered run also lights the schedules view.
	if runs, err := wsp.DB.ListRunningRuns(ctx); err == nil {
		for _, run := range runs {
			st.Task = true
			if run.Trigger == "schedule" {
				st.Schedule = true
			}
		}
	}

	// Flow: any running flow run.
	if fr, err := wsp.DB.ListRunningFlowRuns(ctx); err == nil && len(fr) > 0 {
		st.Flow = true
	}

	writeJSON(w, http.StatusOK, st)
}

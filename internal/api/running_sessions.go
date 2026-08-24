package api

import "github.com/bilal-arikan/tionswarm/internal/workspace"

// runningSessionIDs is the SINGLE answer to "which sessions of this workspace are
// working right now?". Every caller that used to merge the two registries by hand
// (activity flags, the executions feed, the network graph, the in-flight id list)
// goes through here, so a run source is added in one place instead of four.
//
// Three sources, deduped:
//   - s.runs: streamed chat turns. SERVER-WIDE, hence scoped to this workspace —
//     an unscoped read lights an idle workspace with the one we just left.
//   - Runtime.ActiveSessionIDs: autonomous invokes (schedule wake, spawned session,
//     inbox delivery, flow agent node).
//   - Runtime.BusyTurnSessionIDs: every turn holding its session's admission slot.
//     This is the central one — all turn entry paths claim it, including the slash
//     commands (/compact, /handoff) that run on the HTTP goroutine and appear in
//     neither registry.
func (s *Server) runningSessionIDs(wsp *workspace.Workspace) map[string]bool {
	running := map[string]bool{}
	for _, id := range s.runs.activeSessionIDs(wsp.ID) {
		running[id] = true
	}
	for _, id := range wsp.Runtime.ActiveSessionIDs() {
		running[id] = true
	}
	for _, id := range wsp.Runtime.BusyTurnSessionIDs() {
		running[id] = true
	}
	return running
}

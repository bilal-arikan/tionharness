package api

import (
	"context"
	"net/http"

	"github.com/bilal-arikan/tionharness/internal/liveness"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// liveSessions is the SINGLE answer to "which sessions of this workspace are
// working right now?" — the runtime's composed liveness snapshot (_Docs/77 R2).
// Every consumer that used to merge the chat-run registry, the runtime's
// tracked invokes and the turn-slot holders by hand (activity flags, the
// executions feed, the network graph, the in-flight id list, the sidebar chips)
// reads this, so a new liveness source is added in one place
// (agent.Runtime.Liveness) instead of four.
//
// The api's own streamed chat runs (s.runs) are folded in here as well as
// reaching the runtime through the SetExternalActiveSessions probe: the probe is
// wired by the workspace manager, and a Server built without it (tests, tools)
// must still see its own runs. Scoped per workspace — an unscoped read lit an
// idle workspace with the one we just left. Duplicates collapse in the builder.
func (s *Server) liveSessions(wsp *workspace.Workspace) liveness.Snapshot {
	if wsp == nil {
		return liveness.Snapshot{}
	}
	var snap liveness.Snapshot
	if wsp.Runtime != nil {
		// Background context: the snapshot only does two cheap in-memory store
		// listings, and it must not be cancelled by a client that disconnects mid-poll.
		snap = wsp.Runtime.Liveness(context.Background())
	}
	if s.runs != nil {
		snap = snap.WithRunning(s.runs.activeSessionIDs(wsp.ID), "turn:chat")
	}
	return snap
}

// handleWorkspaceLiveness serves the snapshot: running / queued / waiting
// sessions with their reasons, plus spawn capacity and the autonomy brake.
func (s *Server) handleWorkspaceLiveness(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.liveSessions(ws(r)))
}

package api

import (
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// recordQueuedTurnDropped journals a turn that left a session's queue WITHOUT
// ever running. The poison-guard drop already writes a durable error card plus a
// debug event through recordQueueTurnFailure; the removals here are silent by
// design — the user asked for them — so debug.jsonl is the only place the
// disappearance is recorded at all.
//
// source names which queue path removed it (the event's Phase), so a queue that
// keeps emptying itself can be told apart from one the user is pruning
// deliberately. Best effort: a missing workspace/DB writes nothing, an append
// failure is logged rather than swallowed, and neither changes the drop.
func (s *Server) recordQueuedTurnDropped(wsp *workspace.Workspace, sessionID, source string) {
	if wsp == nil || wsp.DB == nil {
		return
	}
	if err := wsp.DB.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:  db.DebugLifecycle,
		Name:  "queued_turn_dropped",
		Phase: source,
	}); err != nil && s.logger != nil {
		s.logger.Error("record queued turn drop debug event failed",
			"session", sessionID, "source", source, "error", err)
	}
}

package api

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
)

// guardCoordinatorChatTurn runs the coordinator phantom-spawn guard for a turn driven
// by an ordinary user chat message.
//
// The runtime-triggered coordinator loop reaches the guard from runCoordinatorTurn, so
// only auto-turns were ever protected: a coordinator answering a USER message could
// narrate "workers started" with no spawn_worker call and nothing looked at the turn
// (WS27/SES90 did exactly that, twice, from a kind="chat" turn whose journal holds no
// tool line at all). This is the chat path's equivalent hook.
//
// The session row is re-read rather than taken from the snapshot the request started
// with, because coordinator mode can be switched on by THIS very turn
// (set_coordinator_mode) — the stale snapshot would say "not a coordinator" for the
// first turn that becomes one. A session that is not a coordinator is left alone: the
// guard is meaningless there and would cost a judge call.
func (s *Server) guardCoordinatorChatTurn(ctx context.Context, database *db.DB, rt *agent.Runtime, sessionID string, agentRow db.Agent, text string, steps []agent.TurnStep) {
	sess, err := database.GetSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("coordinator stall guard skipped: session reload failed",
			"session", sessionID, "error", err)
		return
	}
	if !sess.IsCoordinator() {
		return
	}
	rt.GuardCoordinatorStall(sessionID, agentRow.ID, agentRow, text, steps)
}

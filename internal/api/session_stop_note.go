package api

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// recordAutonomousStop writes a durable transcript card saying a human stopped
// an AUTONOMOUS turn (schedule / wake / spawn / coordination / flow). Those turns
// are cancelled through Runtime.CancelSession, which only kills the context: the
// turn simply stops emitting and, on reload, the log looks like it died on its
// own. The card makes the cause explicit for whoever reads the run later — and
// for analyze-session / the Debug panel, which both index these events.
//
// Best effort: a missing workspace/DB degrades to no card, exactly as the stop
// behaved before.
func (s *Server) recordAutonomousStop(wsp *workspace.Workspace, sessionID string) {
	if wsp == nil || wsp.DB == nil {
		return
	}
	database := wsp.DB
	ctx := context.Background()
	agentID := ""
	if sess, err := database.GetSession(ctx, sessionID); err == nil {
		agentID = sess.AgentID
	}
	// StepError covers cancellation (see agent.StepError) — it renders inline and
	// survives a reload; Reason carries the machine tag.
	step := agent.TurnStep{
		Kind:   agent.StepError,
		Reason: "user_stopped",
		Text:   "Turu kullanıcı durdurdu.",
	}
	msg, err := database.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Steps:     marshalSteps([]agent.TurnStep{step}),
	})
	if err != nil {
		if s.logger != nil {
			s.logger.Error("persist autonomous stop note failed", "session", sessionID, "error", err)
		}
		return
	}
	s.publishHub(wsp.ID, sessionID, sessionhub.KindReply, msg, false)
	if err := database.AppendDebugEventGated(sessionID, db.DebugEvent{
		Type:    db.DebugError,
		AgentID: agentID,
		Name:    "user_stopped",
		Detail:  "autonomous turn cancelled by the user",
	}); err != nil && s.logger != nil {
		s.logger.Error("record autonomous stop debug event failed", "session", sessionID, "error", err)
	}
}

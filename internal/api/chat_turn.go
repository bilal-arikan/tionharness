package api

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/conversation"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/workspace"
)

// isFirstUntitledTurn reports whether this is the opening message of a fresh chat
// session that should auto-generate a title (only when auto-titling is enabled).
// Must be evaluated BEFORE the user message is appended.
func (s *Server) isFirstUntitledTurn(session db.Session) bool {
	return session.Kind == "chat" &&
		strings.TrimSpace(session.Title) == "" && session.MessageCount == 0 &&
		s.settings.Get().AutoTitleEnabled
}

// composeTurnRequest builds the provider request for one chat turn. The system
// prompt is split into a STATIC prefix (user profile + persona + workspace
// instructions) that stays stable across turns so prompt caching remains
// effective, and a DYNAMIC suffix (recalled memory + running summary + existing
// artifacts) that changes every turn and is kept outside the cached prefix.
//
// Shared by both the blocking (chat.go) and streaming (chat_stream.go) handlers.
func (s *Server) composeTurnRequest(ctx context.Context, wsp *workspace.Workspace, session db.Session, agentRow db.Agent, message string, prep conversation.Prepared) providers.Request {
	system := buildSystemPrompt(agentRow)
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if ins := strings.TrimSpace(wsp.Settings().Instructions); ins != "" {
		system = strings.TrimSpace(system + "\n\n# Workspace Instructions\n" + ins)
	}

	var dynamic string
	if block := wsp.Runtime.Memory().ContextBlock(ctx, agentRow.ID, message, 5); block != "" {
		dynamic = block
	}
	if prep.Summary != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n## Conversation summary so far\n" + prep.Summary)
	}
	// Surface the session's existing artifacts so the agent revises them
	// (update_artifact by id) instead of creating duplicates.
	if ab := artifactsContextBlock(ctx, wsp.DB, session.ID); ab != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + ab)
	}

	return providers.Request{
		Model:         agentRow.Model,
		System:        system,
		SystemDynamic: dynamic,
		Messages:      prep.Messages,
	}
}

// marshalSteps serialises the activity trace for persistence. Best-effort: an
// encode error must never fail the reply, so it falls back to an empty trace.
func marshalSteps(steps []agent.TurnStep) string {
	if len(steps) == 0 {
		return "[]"
	}
	if b, err := json.Marshal(steps); err == nil {
		return string(b)
	}
	return "[]"
}

// maybeAutoTitle generates and persists a session title from the opening message
// when firstTurn is set. Best-effort: a failure never breaks the reply. Returns
// the new title, or "" when none was generated.
func (s *Server) maybeAutoTitle(ctx context.Context, wsp *workspace.Workspace, firstTurn bool, agentID, sessionID, message string) string {
	if !firstTurn {
		return ""
	}
	title, err := wsp.Runtime.TitleFor(ctx, agentID, message)
	if err != nil {
		s.logger.Warn("auto title failed", "session", sessionID, "error", err)
		return ""
	}
	if title == "" {
		return ""
	}
	if err := wsp.DB.SetSessionTitle(ctx, sessionID, title); err != nil {
		s.logger.Warn("set session title failed", "session", sessionID, "error", err)
		return ""
	}
	return title
}

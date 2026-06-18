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
func (s *Server) composeTurnRequest(ctx context.Context, wsp *workspace.Workspace, session db.Session, agentRow db.Agent, turnAgents []db.Agent, message string, prep conversation.Prepared, freshSession bool) providers.Request {
	system := buildSystemPrompt(agentRow)
	// Tell the agent its own name and how @mentions work, so a leading "@Name"
	// (the UI's agent selector) is understood as the user addressing this agent —
	// not mistaken for a file, skill, or entity to look up.
	if n := strings.TrimSpace(agentRow.Name); n != "" {
		note := "You are the agent named \"" + n + "\". In this chat, the user picks which agent should answer by starting a message with \"@<AgentName>\". So an \"@" + n + "\" at the start of a message means the user is addressing you by name — treat it as being called, not as a file, skill, or entity to look up; just answer the rest of the message."
		// Multi-agent turn: when the user mentions several agents, each answers the
		// same message in order and later agents can see the earlier replies.
		var others []string
		for _, a := range turnAgents {
			if a.ID != agentRow.ID {
				if nm := strings.TrimSpace(a.Name); nm != "" {
					others = append(others, "@"+nm)
				}
			}
		}
		if len(others) > 0 {
			note += " The user also addressed other agents in this message (" + strings.Join(others, ", ") + "); each mentioned agent answers this same message in turn, and later agents can see the earlier agents' replies. Answer only from your own perspective — do not speak for or impersonate the other agents."
		}
		system = strings.TrimSpace(note + "\n\n" + system)
	}
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if ins := strings.TrimSpace(wsp.Settings().Instructions); ins != "" {
		system = strings.TrimSpace(system + "\n\n# Workspace Instructions\n" + ins)
	}
	// Always-on: deliverables (files/documents) should surface as artifacts.
	system = strings.TrimSpace(system + "\n\n" + artifactDeliverableGuidance)
	// Advertise the skills THIS agent has selected (slug + summary only, in the
	// agent's chosen order). The full body is loaded lazily via use_skill. Part of
	// the cached static prefix since an agent's skill selection changes rarely.
	if sb := wsp.Runtime.SkillsCatalogBlockForAgent(agentRow); sb != "" {
		system = strings.TrimSpace(system + "\n\n" + sb)
	}
	// Advertise the agent's LAZY tools (self-management + MCP) as a lightweight
	// load-on-demand catalog; full schemas are pulled via activate_tools. Part of
	// the cached static prefix since the lazy set is stable per agent/workspace.
	if tb := wsp.Runtime.LazyToolsCatalogBlock(ctx, agentRow); tb != "" {
		system = strings.TrimSpace(system + "\n\n" + tb)
	}

	var dynamic string
	// The session's persistent goal leads the dynamic context — it is the agent's
	// north star and should be the first thing it reads after the static persona.
	if gb := goalContextBlock(session.Goal, session.GoalDone); gb != "" {
		dynamic = gb
	}
	if block := wsp.Runtime.Memory().ContextBlock(ctx, agentRow.ID, message, 5); block != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + block)
	}
	if prep.Summary != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n## Conversation summary so far\n" + prep.Summary)
	}
	// Surface the session's existing artifacts so the agent revises them
	// (update_artifact by id) instead of creating duplicates.
	if ab := artifactsContextBlock(ctx, wsp.DB, session.ID); ab != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + ab)
	}
	// Surface the active todo checklist so the agent keeps tracking it even after
	// the original todo_write message scrolls out of context / is compacted away.
	if tb := todoContextBlock(ctx, wsp.DB, session.ID); tb != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + tb)
	}
	// Cross-session awareness: a short summary of the workspace's active + recent
	// sessions. Configured PER WORKSPACE; injected every turn or only on a
	// session's first turn (its "start") depending on the toggle.
	if sc := wsp.Settings(); sc.SessionContextEnabled && (sc.SessionContextEveryTurn || freshSession) {
		recent := sc.SessionContextRecentCount
		if recent <= 0 {
			recent = 5
		}
		if sb := sessionsContextBlock(ctx, wsp.DB, session.ID, recent); sb != "" {
			dynamic = strings.TrimSpace(dynamic + "\n\n" + sb)
		}
	}

	return providers.Request{
		Model:         agentRow.Model,
		System:        system,
		SystemDynamic: dynamic,
		Messages:      prep.Messages,
	}
}

// adoptMentionedAgent makes the first @mentioned agent the session's default
// (main) agent — but only on a brand-new session (no prior messages), so opening
// a chat by mentioning @X pins the whole thread to X. mentionIDs are the raw
// requested agent ids; agents is the resolved, ordered list (agents[0] is the
// first valid mention). Returns the possibly-updated session.
func (s *Server) adoptMentionedAgent(ctx context.Context, database *db.DB, session db.Session, mentionIDs []string, agents []db.Agent) db.Session {
	if session.MessageCount != 0 || len(mentionIDs) == 0 || len(agents) == 0 {
		return session
	}
	want := agents[0].ID
	if want == "" || want == session.AgentID {
		return session
	}
	if err := database.SetSessionAgent(ctx, session.ID, want); err != nil {
		s.logger.Warn("adopt mentioned agent failed", "session", session.ID, "error", err)
		return session
	}
	session.AgentID = want
	return session
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

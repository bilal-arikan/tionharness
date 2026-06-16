package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

type chatReq struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
	// AgentIDs optionally routes this turn to one or more agents (via "@mention"
	// in the UI). Empty → the session's default agent answers. Multiple → each
	// answers in order, seeing the prior agents' replies.
	AgentIDs []string `json:"agentIds"`
	// ThinkingLevel optionally overrides the responding agent's reasoning level
	// for this single turn ("low" | "medium" | "high" | "off"). Empty = use the
	// agent's own setting. Only affects providers with extended-thinking support
	// (anthropic, non-tool path); others ignore it.
	ThinkingLevel string `json:"thinkingLevel"`
}

type chatResp struct {
	Reply         string           `json:"reply"`
	Usage         providers.Usage  `json:"usage"`
	Model         string           `json:"model"`
	UserMsg       db.Message       `json:"userMessage"`
	ReplyMsg      db.Message       `json:"replyMessage"`
	Steps         []agent.TurnStep `json:"steps,omitempty"`
	ContextTokens int              `json:"contextTokens"`
	Compacted     bool             `json:"compacted"`
	// SessionTitle is set only when the first turn auto-generated a title, so
	// the client can update the session label without an extra round-trip.
	SessionTitle string `json:"sessionTitle,omitempty"`
}

// handleChat runs one turn: persist user message, call the agent's provider
// with full history, persist and return the assistant reply.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.SessionID == "" || strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "sessionId and message are required")
		return
	}

	ctx := r.Context()
	database := ws(r).DB

	session, err := database.GetSession(ctx, req.SessionID)
	// Capture before the user message is appended: an empty title on a fresh
	// chat session means we should auto-generate one from this first message
	// (only when auto-titling is enabled in settings).
	firstTurn := err == nil && session.Kind == "chat" &&
		strings.TrimSpace(session.Title) == "" && session.MessageCount == 0 &&
		s.settings.Get().AutoTitleEnabled
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	agent, err := database.GetAgent(ctx, session.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	// Per-turn reasoning override (local copy only — never persisted).
	if req.ThinkingLevel != "" {
		agent.ThinkingLevel = req.ThinkingLevel
	}

	provider, err := s.providers.Get(agent.Provider)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Persist the incoming user message.
	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      req.Message,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Build conversation history, then budget it to the context window
	// (compacting older turns into the session summary when oversized).
	history, err := database.ListMessages(ctx, session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Reload session so summary state reflects any compaction below.
	session, _ = database.GetSession(ctx, session.ID)
	prep, err := s.convo.Prepare(ctx, database, provider, session, agent, history)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compaction failed: "+err.Error())
		return
	}

	// Compose the system prompt in two parts so prompt caching stays effective:
	// the STATIC prefix (user profile + persona) is stable across turns, while the
	// DYNAMIC suffix (recalled memory + running summary) changes every turn and is
	// kept outside the cached prefix.
	system := buildSystemPrompt(agent)
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if ins := strings.TrimSpace(ws(r).Settings().Instructions); ins != "" {
		system = strings.TrimSpace(system + "\n\n# Workspace Instructions\n" + ins)
	}
	var dynamic string
	if block := ws(r).Runtime.Memory().ContextBlock(ctx, agent.ID, req.Message, 5); block != "" {
		dynamic = block
	}
	if prep.Summary != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n## Conversation summary so far\n" + prep.Summary)
	}
	// Let the agent see the session's existing artifacts so it revises them
	// (update_artifact by id) instead of creating duplicates.
	if ab := artifactsContextBlock(ctx, database, session.ID); ab != "" {
		dynamic = strings.TrimSpace(dynamic + "\n\n" + ab)
	}

	llmReq := providers.Request{
		Model:         agent.Model,
		System:        system,
		SystemDynamic: dynamic,
		Messages:      prep.Messages,
	}

	// Manual chat is not budget-gated (autonomous=false). When the agent has
	// tools enabled this drives the agentic loop (native) or CLI delegation;
	// usage is recorded inside CompleteWithTools. Attach an artifact sink so
	// create_artifact / update_artifact can persist content this turn.
	ctx = tools.WithArtifacts(ctx, newArtifactSink(database, session.ID, agent.ID))
	resp, steps, err := ws(r).Runtime.CompleteWithToolsTraced(ctx, agent, provider, llmReq, false)
	if err != nil {
		s.logger.Error("provider completion failed", "error", err, "agent", agent.ID)
		writeError(w, http.StatusBadGateway, "provider error: "+err.Error())
		return
	}

	// Serialise the activity trace (intermediate text + tool calls) so the
	// turn can be re-rendered on reload. Best-effort: an encode error must not
	// fail the reply, so we fall back to an empty trace.
	stepsJSON := "[]"
	if len(steps) > 0 {
		if b, mErr := json.Marshal(steps); mErr == nil {
			stepsJSON = string(b)
		}
	}

	// Persist the assistant reply.
	replyMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   agent.ID,
		Text:      resp.Text,
		Steps:     stepsJSON,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Usage already recorded inside CompleteWithTools; just journal the turn.
	ws(r).Runtime.Journal(ctx, agent.ID, "Q: "+req.Message+"\nA: "+resp.Text)

	// On the first turn of an untitled chat, auto-generate a title from the
	// opening message. Best-effort: a failure must never break the reply.
	var sessionTitle string
	if firstTurn {
		if title, err := ws(r).Runtime.TitleFor(ctx, agent.ID, req.Message); err == nil && title != "" {
			if err := database.SetSessionTitle(ctx, session.ID, title); err == nil {
				sessionTitle = title
			} else {
				s.logger.Warn("set session title failed", "session", session.ID, "error", err)
			}
		} else if err != nil {
			s.logger.Warn("auto title failed", "session", session.ID, "error", err)
		}
	}

	writeJSON(w, http.StatusOK, chatResp{
		Reply:         resp.Text,
		Usage:         resp.Usage,
		Model:         resp.Model,
		UserMsg:       userMsg,
		ReplyMsg:      replyMsg,
		Steps:         steps,
		ContextTokens: prep.ContextTokens,
		Compacted:     prep.Compacted,
		SessionTitle:  sessionTitle,
	})
}

// buildSystemPrompt composes the agent's system prompt from soul + identity.
func buildSystemPrompt(a db.Agent) string {
	var b strings.Builder
	if a.Soul != "" {
		b.WriteString(a.Soul)
		b.WriteString("\n\n")
	}
	if a.Identity != "" {
		b.WriteString(a.Identity)
	}
	return strings.TrimSpace(b.String())
}

package api

import (
	"net/http"
	"strings"
	"time"

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
	// PermissionMode optionally overrides the responding agent's tool-use
	// permission gate for this single turn ("read-only" | "ask" | "auto").
	// Empty = use the agent's own setting.
	PermissionMode string `json:"permissionMode"`
	// Attachments are files (or pasted long text) sent with this turn, already
	// uploaded via POST /api/uploads. Persisted on the user message and folded
	// into the provider request.
	Attachments []db.Attachment `json:"attachments"`
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
	if req.SessionID == "" || (strings.TrimSpace(req.Message) == "" && len(req.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "sessionId and message (or attachments) are required")
		return
	}

	ctx := r.Context()
	database := ws(r).DB
	start := time.Now()

	session, err := database.GetSession(ctx, req.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	// Capture before the user message is appended: an empty title on a fresh
	// chat session means we should auto-generate one from this first message
	// (only when auto-titling is enabled in settings).
	firstTurn := s.isFirstUntitledTurn(session)
	agent, err := database.GetAgent(ctx, session.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	// Per-turn reasoning override (local copy only — never persisted).
	if req.ThinkingLevel != "" {
		agent.ThinkingLevel = req.ThinkingLevel
	}
	if req.PermissionMode != "" {
		agent.PermissionMode = req.PermissionMode
	}

	provider, err := s.providers.Get(agent.Provider)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Persist the incoming user message.
	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID:   session.ID,
		Role:        providers.RoleUser,
		Text:        req.Message,
		Attachments: req.Attachments,
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

	llmReq := s.composeTurnRequest(ctx, ws(r), session, agent, req.Message, prep)

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

	// Persist the assistant reply (with its serialised activity trace so the
	// turn can be re-rendered on reload).
	replyMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		AgentID:   agent.ID,
		Text:      resp.Text,
		Steps:     marshalSteps(steps),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Usage already recorded inside CompleteWithTools; just journal the turn.
	ws(r).Runtime.Journal(ctx, agent.ID, "Q: "+req.Message+"\nA: "+resp.Text)

	// Auto-capture any files the agent wrote this turn as artifacts.
	s.captureFileArtifacts(ctx, database, session.ID, agent.ID, steps)

	s.logger.Info("chat turn completed",
		"session", session.ID, "agent", agent.Name, "provider", agent.Provider,
		"model", resp.Model, "in", resp.Usage.InputTokens, "out", resp.Usage.OutputTokens,
		"steps", len(steps), "dur", time.Since(start).Round(time.Millisecond).String())

	// On the first turn of an untitled chat, auto-generate a title from the
	// opening message. Best-effort: a failure must never break the reply.
	sessionTitle := s.maybeAutoTitle(ctx, ws(r), firstTurn, agent.ID, session.ID, req.Message)

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

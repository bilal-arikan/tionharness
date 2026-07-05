package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// messageUsage converts a provider Usage into the compact per-message form stored
// on the assistant turn, returning nil when the turn reported no tokens (so an
// empty/non-LLM turn doesn't carry a zero usage object).
func messageUsage(u providers.Usage) *db.MessageUsage {
	if u.InputTokens == 0 && u.OutputTokens == 0 && u.CacheReadTokens == 0 && u.CacheWriteTokens == 0 {
		return nil
	}
	return &db.MessageUsage{
		InputTokens:      u.InputTokens,
		OutputTokens:     u.OutputTokens,
		CacheReadTokens:  u.CacheReadTokens,
		CacheWriteTokens: u.CacheWriteTokens,
	}
}

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
	req, ok := bindJSON[chatReq](w, r)
	if !ok {
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
	// A coordinator session runs at most ONE turn at a time: claim the turn slot
	// (blocking until any in-flight auto turn finishes) so this interactive turn
	// never overlaps an auto-triggered coordinator turn. Worker notifications
	// arriving mid-turn coalesce and trigger one auto turn on release.
	if session.Role == "coordinator" {
		release := ws(r).Runtime.BeginCoordinatorUserTurn(session.ID)
		defer release()
	}
	// Capture before the user message is appended: an empty title on a fresh
	// chat session means we should auto-generate one from this first message
	// (only when auto-titling is enabled in settings).
	firstTurn := s.isFirstUntitledTurn(session)
	// Capture before the user message is appended: a fresh session (no prior
	// messages) gets the cross-session context primed on its first turn.
	freshSession := session.MessageCount == 0
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

	// Persist the incoming user message. Stamp the recipient agent so a multi-agent
	// thread's history can show which agent each question was directed at (the
	// "@name" in the text is only informational). Harmless in a 1:1 session.
	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID:   session.ID,
		Role:        providers.RoleUser,
		AgentID:     agent.ID,
		Text:        req.Message,
		Attachments: req.Attachments,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Every file attached to a chat turn becomes a session artifact (origin chat).
	s.captureAttachmentArtifacts(ctx, database, session.ID, agent.ID, req.Attachments)

	// Build conversation history, then budget it to the context window
	// (compacting older turns into the session summary when oversized).
	history, err := database.ListMessages(ctx, session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Reload session so summary state reflects any compaction below.
	session, _ = database.GetSession(ctx, session.ID)
	// Annotate history with each assistant turn's author so this agent can tell who
	// said what when several agents share the thread (no-op for a 1:1 session).
	history, multiAgent := s.labelMultiAgentHistory(ctx, database, agent.ID, history)
	// Recap recent turns' tool I/O so the agent can answer "what did you just do /
	// what did that return" (the tool trace is dropped when history → messages).
	history = appendRecentToolSummaries(history)
	// Carry this workspace's editable compaction prompt onto the turn context so
	// any fold (rolling summary here, or reactive mid-loop downstream) uses it.
	ctx = conversation.WithCompactPrompt(ctx, ws(r).Runtime.CompactPromptTemplate())
	prep, err := s.convo.Prepare(ctx, database, provider, session, agent, history)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compaction failed: "+err.Error())
		return
	}

	llmReq := s.composeTurnRequest(ctx, ws(r), session, agent, []db.Agent{agent}, req.Message, prep, freshSession, multiAgent)

	// Manual chat is not budget-gated (autonomous=false). When the agent has
	// tools enabled this drives the agentic loop (native) or CLI delegation;
	// usage is recorded inside CompleteWithTools. Attach an artifact sink so
	// create_artifact / update_artifact can persist content this turn.
	ctx = tools.WithGrants(ctx, s.grants.forSession(session.ID))
	ctx = tools.WithArtifacts(ctx, newArtifactSink(database, session.ID, agent.ID, ws(r).Runtime.Emit))
	ctx = withSessionID(ctx, session.ID) // resolve this session's WorkingDir downstream
	resp, steps, err := ws(r).Runtime.CompleteWithToolsTraced(ctx, agent, provider, llmReq, false)
	if err != nil {
		s.logger.Error("provider completion failed", "error", err, "agent", agent.ID)
		writeError(w, http.StatusBadGateway, "provider error: "+err.Error())
		return
	}

	// Persist the assistant reply (with its serialised activity trace so the
	// turn can be re-rendered on reload).
	replyMsg, err := database.AddMessage(ctx, db.Message{
		SessionID:  session.ID,
		Role:       providers.RoleAssistant,
		AgentID:    agent.ID,
		Text:       resp.Text,
		Steps:      marshalSteps(steps),
		Model:      resp.Model,
		StopReason: resp.StopReason,
		Usage:      messageUsage(resp.Usage),
		DurationMs: time.Since(start).Milliseconds(),
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

// buildSystemPrompt composes the agent's system prompt from soul + identity, plus
// the shared goal-usage hint in the cached static prefix (so chat turns nudge the
// agent to use set_session_goal/complete_goal without loading a skill). Mirrors
// the agent package's assembler; both append the same agent.GoalUsageHint.
func buildSystemPrompt(a db.Agent) string {
	var b strings.Builder
	if a.Soul != "" {
		b.WriteString(a.Soul)
		b.WriteString("\n\n")
	}
	if a.Identity != "" {
		b.WriteString(a.Identity)
		b.WriteString("\n\n")
	}
	b.WriteString(agent.GoalUsageHint)
	return strings.TrimSpace(b.String())
}

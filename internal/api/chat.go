package api

import (
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

type chatReq struct {
	SessionID string `json:"sessionId"`
	Message   string `json:"message"`
}

type chatResp struct {
	Reply         string          `json:"reply"`
	Usage         providers.Usage `json:"usage"`
	Model         string          `json:"model"`
	UserMsg       db.Message      `json:"userMessage"`
	ReplyMsg      db.Message      `json:"replyMessage"`
	ContextTokens int             `json:"contextTokens"`
	Compacted     bool            `json:"compacted"`
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
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	agent, err := database.GetAgent(ctx, session.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
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

	// Compose the system prompt: persona + recalled memory + rolling summary.
	system := buildSystemPrompt(agent)
	if block := ws(r).Runtime.Memory().ContextBlock(ctx, agent.ID, req.Message, 5); block != "" {
		system = strings.TrimSpace(system + "\n\n" + block)
	}
	if prep.Summary != "" {
		system = strings.TrimSpace(system + "\n\n## Conversation summary so far\n" + prep.Summary)
	}

	llmReq := providers.Request{
		Model:    agent.Model,
		System:   system,
		Messages: prep.Messages,
	}

	resp, err := provider.Complete(ctx, llmReq)
	if err != nil {
		s.logger.Error("provider completion failed", "error", err, "agent", agent.ID)
		writeError(w, http.StatusBadGateway, "provider error: "+err.Error())
		return
	}

	// Persist the assistant reply.
	replyMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		Text:      resp.Text,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Record token usage (manual chat is not budget-gated) and journal the turn.
	ws(r).Runtime.RecordUsage(ctx, agent.ID, resp.Usage)
	ws(r).Runtime.Journal(ctx, agent.ID, "Q: "+req.Message+"\nA: "+resp.Text)

	writeJSON(w, http.StatusOK, chatResp{
		Reply:         resp.Text,
		Usage:         resp.Usage,
		Model:         resp.Model,
		UserMsg:       userMsg,
		ReplyMsg:      replyMsg,
		ContextTokens: prep.ContextTokens,
		Compacted:     prep.Compacted,
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

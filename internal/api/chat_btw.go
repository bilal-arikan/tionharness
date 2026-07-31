package api

import (
	"net/http"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
)

// btwReq is one side-chat ("btw") question. It carries no attachments and no
// permission mode on purpose: the side chat has no tools, so there is nothing
// for a permission mode to gate.
type btwReq struct {
	SessionID string `json:"sessionId"`
	Question  string `json:"question"`
	// AgentID optionally picks WHICH agent answers (the composer's agent
	// dropdown). Empty → the session's default agent.
	AgentID string `json:"agentId"`
}

// btwResp is the side-chat answer. It is returned to the caller and NOT
// persisted anywhere — there is no message id, because no message was created.
type btwResp struct {
	Answer string          `json:"answer"`
	Model  string          `json:"model"`
	Usage  providers.Usage `json:"usage"`
}

// handleChatBtw answers a side question against the session's context WITHOUT
// touching the session: neither the question nor the answer is appended to the
// history, no artifacts are captured, no turn is auto-tagged, no title is
// generated. See _Docs/60-BTW-YAN-SOHBET.md.
//
// It deliberately does NOT go through the runtime's tool loop
// (CompleteWithTools): a single tool-less provider call is the whole point, so
// the side chat can never edit a file or run a command. It also does not claim
// the session's turn slot — a btw question is answerable WHILE the main turn is
// still streaming, which is the feature's reason to exist ("ana görevi kesmez").
func (s *Server) handleChatBtw(w http.ResponseWriter, r *http.Request) {
	req, ok := bindJSON[btwReq](w, r)
	if !ok {
		return
	}
	if req.SessionID == "" || strings.TrimSpace(req.Question) == "" {
		writeError(w, http.StatusBadRequest, "sessionId and question are required")
		return
	}

	ctx := r.Context()
	database := ws(r).DB

	session, err := database.GetSession(ctx, req.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		agentID = session.AgentID
	}
	agentRow, err := database.GetAgent(ctx, agentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	provider, err := s.providers.Get(agentRow.Provider)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	// Read the SAME history the next real turn would send, so the side question
	// sees the files the agent read and the decisions it made. Nothing is appended
	// first: the question is not a session message.
	history, err := database.ListMessages(ctx, session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	history, multiAgent := s.labelMultiAgentHistory(ctx, database, agentRow.ID, history)
	toolRecap := recentToolActivityBlock(history)
	feedbackRecap := recentFeedbackBlock(history)

	// Budget the history to the context window exactly as a real turn does. NOTE:
	// Prepare may fold older turns into the session's rolling summary — that is a
	// legitimate shared side effect (the same fold the next real turn would have
	// performed), not a side-chat mutation of the transcript.
	ctx = conversation.WithCompactPrompt(ctx, ws(r).Runtime.CompactPromptTemplate())
	ctx = conversation.WithAttachmentRoot(ctx, ws(r).SandboxRoot())
	prep, err := s.convo.Prepare(ctx, database, provider, session, agentRow, history)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compaction failed: "+err.Error())
		return
	}

	// Compose the turn's context (static prefix + volatile suffix) with the SAME
	// builder the real turn uses, so the side chat is not a second, drifting copy
	// of the context assembly. freshSession=false: a side question is never a
	// session's opening turn. lifecycleContext="": UserPromptSubmit hooks are for
	// real user turns; a btw question is not one.
	llmReq := s.composeTurnRequest(ctx, ws(r), session, agentRow, []db.Agent{agentRow}, req.Question, prep, false, multiAgent, toolRecap, feedbackRecap, "")

	answer, err := ws(r).Runtime.AskBtw(ctx, agentRow, session.ID, llmReq.System, llmReq.SystemDynamic, llmReq.Messages, req.Question)
	if err != nil {
		s.logger.Error("btw side chat failed", "session", session.ID, "agent", agentRow.ID, "error", err)
		writeError(w, http.StatusBadGateway, "provider error: "+err.Error())
		return
	}

	s.logger.Info("btw side chat answered",
		"session", session.ID, "agent", agentRow.Name, "model", answer.Model,
		"in", answer.Usage.InputTokens, "out", answer.Usage.OutputTokens)

	writeJSON(w, http.StatusOK, btwResp{
		Answer: answer.Text,
		Model:  answer.Model,
		Usage:  answer.Usage,
	})
}

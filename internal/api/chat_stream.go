package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// handleChatStream runs one chat turn over Server-Sent Events, emitting each
// activity step (text/thinking/tool) the moment it occurs so the UI can render
// the turn step-by-step. Event types:
//
//	meta  → { userMessage, contextTokens }   (once, before steps)
//	step  → a single TurnStep                (zero or more, in order)
//	done  → { replyMessage, model, usage, sessionTitle }  (terminal, success)
//	error → { error }                        (terminal, failure)
//
// Pre-flight failures (bad input, not found) use a normal JSON error response;
// once streaming has begun, errors are delivered as an `error` event.
func (s *Server) handleChatStream(w http.ResponseWriter, r *http.Request) {
	var req chatReq
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.SessionID == "" || strings.TrimSpace(req.Message) == "" {
		writeError(w, http.StatusBadRequest, "sessionId and message are required")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ctx := r.Context()
	database := ws(r).DB

	session, err := database.GetSession(ctx, req.SessionID)
	firstTurn := err == nil && session.Kind == "chat" &&
		strings.TrimSpace(session.Title) == "" && session.MessageCount == 0 &&
		s.settings.Get().AutoTitleEnabled
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	agentRow, err := database.GetAgent(ctx, session.AgentID)
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	provider, err := s.providers.Get(agentRow.Provider)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      req.Message,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	history, err := database.ListMessages(ctx, session.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	session, _ = database.GetSession(ctx, session.ID)
	prep, err := s.convo.Prepare(ctx, database, provider, session, agentRow, history)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compaction failed: "+err.Error())
		return
	}

	system := buildSystemPrompt(agentRow)
	if uc := userContextBlock(s.settings.Get()); uc != "" {
		system = strings.TrimSpace(uc + "\n\n" + system)
	}
	if block := ws(r).Runtime.Memory().ContextBlock(ctx, agentRow.ID, req.Message, 5); block != "" {
		system = strings.TrimSpace(system + "\n\n" + block)
	}
	if prep.Summary != "" {
		system = strings.TrimSpace(system + "\n\n## Conversation summary so far\n" + prep.Summary)
	}

	llmReq := providers.Request{
		Model:    agentRow.Model,
		System:   system,
		Messages: prep.Messages,
	}

	// Begin the event stream.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // disable proxy buffering
	w.WriteHeader(http.StatusOK)

	sse := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	sse("meta", map[string]any{
		"userMessage":   userMsg,
		"contextTokens": prep.ContextTokens,
	})

	// Stream the turn: each step is delivered as it becomes available.
	resp, steps, err := ws(r).Runtime.CompleteWithToolsStream(ctx, agentRow, provider, llmReq, false,
		func(st agent.TurnStep) { sse("step", st) },
	)
	if err != nil {
		s.logger.Error("stream completion failed", "error", err, "agent", agentRow.ID)
		sse("error", map[string]string{"error": "provider error: " + err.Error()})
		return
	}

	stepsJSON := "[]"
	if len(steps) > 0 {
		if b, mErr := json.Marshal(steps); mErr == nil {
			stepsJSON = string(b)
		}
	}

	replyMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleAssistant,
		Text:      resp.Text,
		Steps:     stepsJSON,
	})
	if err != nil {
		sse("error", map[string]string{"error": err.Error()})
		return
	}

	ws(r).Runtime.Journal(ctx, agentRow.ID, "Q: "+req.Message+"\nA: "+resp.Text)

	var sessionTitle string
	if firstTurn {
		if title, terr := ws(r).Runtime.TitleFor(ctx, agentRow.ID, req.Message); terr == nil && title != "" {
			if serr := database.SetSessionTitle(ctx, session.ID, title); serr == nil {
				sessionTitle = title
			}
		}
	}

	sse("done", map[string]any{
		"replyMessage": replyMsg,
		"model":        resp.Model,
		"usage":        resp.Usage,
		"sessionTitle": sessionTitle,
	})
}

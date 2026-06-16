package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
)

// handleChatStream runs one chat turn over Server-Sent Events, emitting each
// activity step the moment it occurs. A turn may involve several agents (via
// "@mention" routing); each answers in order, seeing the prior replies. Events:
//
//	meta  → { userMessage, contextTokens }      (once)
//	agent → { agentId, index }                  (before each agent's steps)
//	step  → a single TurnStep                   (zero or more, current agent)
//	reply → { replyMessage }                    (after each agent finishes)
//	done  → { sessionTitle }                    (terminal, success)
//	error → { error }                           (terminal, failure)
//
// Pre-flight failures use a normal JSON error; once streaming begins, errors
// are delivered as an `error` event.
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

	// Register this turn so it can be stopped or steered while running. The
	// cancelable context ends the stream on "stop"; the steer channel feeds live
	// guidance into the tool loop.
	runID := uuid.NewString()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	run := s.runs.register(runID, cancel)
	defer s.runs.unregister(runID)
	ctx = agent.WithSteer(ctx, run.steer)

	wsp := ws(r)
	database := wsp.DB

	session, err := database.GetSession(ctx, req.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	firstTurn := session.Kind == "chat" &&
		strings.TrimSpace(session.Title) == "" && session.MessageCount == 0 &&
		s.settings.Get().AutoTitleEnabled

	// Resolve the ordered list of responding agents (default → session agent).
	agents := s.resolveTurnAgents(ctx, database, session, req.AgentIDs)
	if len(agents) == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}

	// Persist the incoming user message once.
	userMsg, err := database.AddMessage(ctx, db.Message{
		SessionID: session.ID,
		Role:      providers.RoleUser,
		Text:      req.Message,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Begin the event stream.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	sse := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	}

	sse("meta", map[string]any{"userMessage": userMsg, "runId": runID})

	// Each agent answers in turn, re-reading the (growing) history so later
	// agents see the earlier replies.
	for i, agentRow := range agents {
		provider, perr := s.providers.Get(agentRow.Provider)
		if perr != nil {
			sse("error", map[string]string{"error": perr.Error()})
			return
		}

		sse("agent", map[string]any{"agentId": agentRow.ID, "index": i})

		history, herr := database.ListMessages(ctx, session.ID)
		if herr != nil {
			sse("error", map[string]string{"error": herr.Error()})
			return
		}
		session, _ = database.GetSession(ctx, session.ID)
		prep, cerr := s.convo.Prepare(ctx, database, provider, session, agentRow, history)
		if cerr != nil {
			sse("error", map[string]string{"error": "compaction failed: " + cerr.Error()})
			return
		}

		system := buildSystemPrompt(agentRow)
		if uc := userContextBlock(s.settings.Get()); uc != "" {
			system = strings.TrimSpace(uc + "\n\n" + system)
		}
		if block := wsp.Runtime.Memory().ContextBlock(ctx, agentRow.ID, req.Message, 5); block != "" {
			system = strings.TrimSpace(system + "\n\n" + block)
		}
		if prep.Summary != "" {
			system = strings.TrimSpace(system + "\n\n## Conversation summary so far\n" + prep.Summary)
		}

		llmReq := providers.Request{Model: agentRow.Model, System: system, Messages: prep.Messages}

		resp, steps, cerr := wsp.Runtime.CompleteWithToolsStream(ctx, agentRow, provider, llmReq, false,
			func(st agent.TurnStep) { sse("step", st) },
		)
		if cerr != nil {
			s.logger.Error("stream completion failed", "error", cerr, "agent", agentRow.ID)
			sse("error", map[string]string{"error": "provider error: " + cerr.Error()})
			return
		}

		stepsJSON := "[]"
		if len(steps) > 0 {
			if b, mErr := json.Marshal(steps); mErr == nil {
				stepsJSON = string(b)
			}
		}

		replyMsg, aerr := database.AddMessage(ctx, db.Message{
			SessionID: session.ID,
			Role:      providers.RoleAssistant,
			AgentID:   agentRow.ID,
			Text:      resp.Text,
			Steps:     stepsJSON,
		})
		if aerr != nil {
			sse("error", map[string]string{"error": aerr.Error()})
			return
		}
		wsp.Runtime.Journal(ctx, agentRow.ID, "Q: "+req.Message+"\nA: "+resp.Text)
		sse("reply", map[string]any{"replyMessage": replyMsg})
	}

	// Auto-title once, after the turn, using the first responding agent.
	var sessionTitle string
	if firstTurn {
		if title, terr := wsp.Runtime.TitleFor(ctx, agents[0].ID, req.Message); terr == nil && title != "" {
			if serr := database.SetSessionTitle(ctx, session.ID, title); serr == nil {
				sessionTitle = title
			}
		}
	}

	sse("done", map[string]any{"sessionTitle": sessionTitle})
}

// resolveTurnAgents turns the requested agent ids (from "@mention" routing) into
// an ordered, de-duplicated list of existing agents. Falls back to the session's
// default agent when none are given or valid.
func (s *Server) resolveTurnAgents(ctx context.Context, database *db.DB, session db.Session, ids []string) []db.Agent {
	var out []db.Agent
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		if a, err := database.GetAgent(ctx, id); err == nil {
			seen[id] = true
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		if a, err := database.GetAgent(ctx, session.AgentID); err == nil {
			out = append(out, a)
		}
	}
	return out
}

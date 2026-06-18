package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
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
	if req.SessionID == "" || (strings.TrimSpace(req.Message) == "" && len(req.Attachments) == 0) {
		writeError(w, http.StatusBadRequest, "sessionId and message (or attachments) are required")
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
	// Detach the turn from the client connection. A page refresh or navigation
	// aborts the SSE fetch; if generation were tied to the request context it
	// would cancel mid-turn and the assistant reply would never be persisted —
	// so after reload the answer is gone and the message block "vanishes". By
	// detaching, generation runs to completion and persists regardless; only an
	// explicit "stop" control cancels it. The original request context is kept
	// as clientGone so an interactive ask_user (which needs a live client) does
	// not block forever once the user has navigated away.
	clientGone := r.Context()
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	defer cancel()
	run := s.runs.register(runID, req.SessionID, cancel)
	defer s.runs.unregister(runID)
	ctx = agent.WithSteer(ctx, run.steer)
	// Point any CLI subprocess (claude-cli, ...) at the in-process Interaction MCP
	// endpoint for this turn, carrying the per-run token so its ask_user/todo_write
	// calls correlate back here. No-op when the base URL is unknown.
	if url := s.interactionURL(); url != "" {
		ctx = tools.WithInteractionEndpoint(ctx, url, run.token)
	}

	wsp := ws(r)
	database := wsp.DB
	// Drop any crash-recovery sidecar when the turn returns by any normal path
	// (success, handled failure, client abort): only a true mid-turn process
	// death must leave it behind for the next boot to reclaim.
	defer database.ClearInflight(req.SessionID)

	// Ensure a terminal chat event fires even when generation fails after the
	// turn has begun: the frontend uses it to clear the post-reload "thinking"
	// indicator and reload the transcript. The success path sets emitted=true
	// and publishes its own richer event below.
	turnStarted := false
	emitted := false
	defer func() {
		if !turnStarted || emitted {
			return
		}
		wsp.Runtime.Emit(events.Event{
			Type:   "chat",
			Level:  "error",
			Title:  "Sohbet turu sonlandı",
			Target: map[string]string{"view": "chat", "sessionId": req.SessionID},
		})
	}()

	session, err := database.GetSession(ctx, req.SessionID)
	if err != nil {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	firstTurn := s.isFirstUntitledTurn(session)
	// Captured before the user message is appended: primes cross-session context
	// on a fresh session's first turn.
	freshSession := session.MessageCount == 0

	// Resolve the ordered list of responding agents (default → session agent).
	agents := s.resolveTurnAgents(ctx, database, session, req.AgentIDs)
	if len(agents) == 0 {
		writeError(w, http.StatusNotFound, "agent not found")
		return
	}
	// A fresh session opened by @mentioning an agent adopts it as the main agent.
	session = s.adoptMentionedAgent(ctx, database, session, req.AgentIDs, agents)

	// Persist the incoming user message once.
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
	// The turn is now committed (user message persisted); arm the terminal-event
	// guard so a later failure still notifies the frontend.
	turnStarted = true
	// Every file attached to a chat turn becomes a session artifact (origin chat).
	s.captureAttachmentArtifacts(ctx, database, session.ID, agents[0].ID, req.Attachments)

	// Begin the event stream.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// Route every SSE write through the run's mutex-guarded writer so the stream
	// handler goroutine and the Interaction MCP handler goroutine (which emits
	// ask/todo steps for the CLI path) never race on the ResponseWriter.
	run.setWrite(func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		flusher.Flush()
	})
	defer run.clearWrite()
	sse := run.emit

	sse("meta", map[string]any{"userMessage": userMsg, "runId": runID})

	// Wire the interactive asker: the ask_user tool emits a transient "ask" step
	// and blocks here until the client POSTs an answer (or the turn is stopped).
	// The tool loop runs in this same goroutine, so emitting via sse is safe.
	ctx = tools.WithAsker(ctx, func(ctx context.Context, question string, options []string) (string, error) {
		sse("step", agent.TurnStep{Kind: agent.StepAsk, Text: question, Options: options})
		select {
		case ans := <-run.answer:
			return ans, nil
		case <-clientGone.Done():
			// The user navigated away; no one can answer. Surface an error so the
			// model proceeds on its own instead of blocking the detached turn
			// forever (and leaking this goroutine).
			return "", clientGone.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// Session-scoped permission grants + the approval prompter for write/exec
	// tools under "ask" mode. The prompter emits a dedicated StepPermission card
	// and blocks on the same answer channel as ask_user; "Always allow" is
	// recorded in the session grants so it isn't re-asked. Also wired onto the run
	// so the CLI permission-prompt tool shares the same grant set.
	grants := s.grants.forSession(session.ID)
	run.setGrants(grants)
	ctx = tools.WithGrants(ctx, grants)
	ctx = tools.WithPermissionPrompter(ctx, func(ctx context.Context, tool, risk string, options []string) (string, error) {
		sse("step", agent.TurnStep{Kind: agent.StepPermission, Tool: tool, Reason: risk, Options: options})
		select {
		case ans := <-run.answer:
			return ans, nil
		case <-clientGone.Done():
			return "", clientGone.Err()
		case <-ctx.Done():
			return "", ctx.Err()
		}
	})

	// Each agent answers in turn, re-reading the (growing) history so later
	// agents see the earlier replies.
	for i, agentRow := range agents {
		// Per-turn reasoning override (local copy only — never persisted).
		if req.ThinkingLevel != "" {
			agentRow.ThinkingLevel = req.ThinkingLevel
		}
		if req.PermissionMode != "" {
			agentRow.PermissionMode = req.PermissionMode
		}
		provider, perr := s.providers.Get(agentRow.Provider)
		if perr != nil {
			s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "provider_unavailable", perr.Error())
			return
		}

		sse("agent", map[string]any{"agentId": agentRow.ID, "index": i})

		history, herr := database.ListMessages(ctx, session.ID)
		if herr != nil {
			s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "history_error", herr.Error())
			return
		}
		session, _ = database.GetSession(ctx, session.ID)
		prep, cerr := s.convo.Prepare(ctx, database, provider, session, agentRow, history)
		if cerr != nil {
			s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "compaction_failed", "compaction failed: "+cerr.Error())
			return
		}

		llmReq := s.composeTurnRequest(ctx, wsp, session, agentRow, agents, req.Message, prep, freshSession)

		// Attach a per-agent artifact sink so create_artifact / update_artifact
		// persist content stamped with this session + agent — both on the native
		// tool path (via context) and the CLI path (via the run, used by the
		// Interaction MCP backend).
		sink := newArtifactSink(database, session.ID, agentRow.ID)
		run.setArtifacts(sink)
		turnCtx := tools.WithArtifacts(ctx, sink)

		agentStart := time.Now()
		// Pre-allocate the reply id so the streaming crash sidecar and the final
		// persisted message share one identity (recovery is then idempotent).
		replyID := uuid.NewString()
		// Snapshot the in-flight reply to disk on a throttle: the partial answer
		// text (accumulated from streaming deltas) plus the persistable trace so
		// far. A mid-turn process death leaves this sidecar for boot to reclaim.
		var partial strings.Builder
		var kept []agent.TurnStep
		var lastSnap time.Time
		snapshot := func() {
			if time.Since(lastSnap) < 600*time.Millisecond {
				return
			}
			lastSnap = time.Now()
			_ = database.WriteInflight(db.InflightTurn{
				MessageID: replyID,
				SessionID: session.ID,
				AgentID:   agentRow.ID,
				StartedAt: agentStart.Unix(),
				Text:      partial.String(),
				Steps:     marshalSteps(kept),
			})
		}
		resp, steps, cerr := wsp.Runtime.CompleteWithToolsStream(turnCtx, agentRow, provider, llmReq, false,
			func(st agent.TurnStep) {
				sse("step", st)
				switch st.Kind {
				case agent.StepDelta:
					partial.WriteString(st.Text)
				case agent.StepAsk, agent.StepToolDelta, agent.StepTombstone, agent.StepPermission:
					// Transient (live-UI only) — never part of the persisted trace.
				default:
					kept = append(kept, st)
				}
				snapshot()
			},
		)
		if cerr != nil {
			s.logger.Error("stream completion failed", "error", cerr, "agent", agentRow.ID)
			s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "provider_error", "provider error: "+cerr.Error())
			return
		}

		replyMsg, aerr := database.AddMessage(ctx, db.Message{
			ID:        replyID,
			SessionID: session.ID,
			Role:      providers.RoleAssistant,
			AgentID:   agentRow.ID,
			Text:      resp.Text,
			Steps:     marshalSteps(steps),
		})
		if aerr != nil {
			s.failTurn(ctx, database, sse, session.ID, agentRow.ID, "persist_error", aerr.Error())
			return
		}
		// Reply is durable now; drop this agent's sidecar before the next agent
		// (the top-level defer is the catch-all for early-return paths).
		_ = database.ClearInflight(session.ID)
		wsp.Runtime.Journal(ctx, agentRow.ID, "Q: "+req.Message+"\nA: "+resp.Text)
		// Auto-capture any files the agent wrote this turn as artifacts.
		s.captureFileArtifacts(ctx, database, session.ID, agentRow.ID, steps)
		sse("reply", map[string]any{"replyMessage": replyMsg})

		s.logger.Info("chat turn completed",
			"session", session.ID, "agent", agentRow.Name, "provider", agentRow.Provider,
			"model", resp.Model, "in", resp.Usage.InputTokens, "out", resp.Usage.OutputTokens,
			"steps", len(steps), "stream", true,
			"dur", time.Since(agentStart).Round(time.Millisecond).String())
	}

	// Auto-title once, after the turn, using the first responding agent.
	sessionTitle := s.maybeAutoTitle(ctx, wsp, firstTurn, agents[0].ID, session.ID, req.Message)

	sse("done", map[string]any{"sessionTitle": sessionTitle})

	// Publish a chat-completion event so other workspaces can flag activity with
	// a badge when the user is viewing a different workspace. The frontend uses
	// chat events only for the badge (not a duplicate desktop notification).
	title := strings.TrimSpace(session.Title)
	if sessionTitle != "" {
		title = sessionTitle
	}
	if title == "" {
		title = "Sohbet"
	}
	emitted = true
	wsp.Runtime.Emit(events.Event{
		Type:   "chat",
		Level:  "success",
		Title:  "Yanıt hazır: " + title,
		Body:   req.Message,
		Target: map[string]string{"view": "chat", "sessionId": session.ID},
	})
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

// failTurn records a turn-level failure as a persisted assistant message so the
// error shows up in the chat hierarchy (with its detail) and survives a reload,
// then notifies the client via the SSE `error` event carrying that message.
// agentID is the responding agent (may be "" if none was selected yet); reason
// is a stable machine tag (provider_error, compaction_failed, …) shown as a
// badge; detail is the human-readable message.
func (s *Server) failTurn(ctx context.Context, database *db.DB, sse func(string, any), sessionID, agentID, reason, detail string) {
	step := agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason}
	payload := map[string]any{"error": detail, "reason": reason}
	if msg, err := database.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		Role:      providers.RoleAssistant,
		AgentID:   agentID,
		Steps:     marshalSteps([]agent.TurnStep{step}),
	}); err != nil {
		s.logger.Error("persist turn error failed", "session", sessionID, "error", err)
	} else {
		payload["replyMessage"] = msg
	}
	sse("error", payload)
}

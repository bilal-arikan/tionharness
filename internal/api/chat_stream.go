package api

import (
	"context"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
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
// runChatTurn runs one chat turn to completion, publishing every UI event to the
// session hub (the authoritative render path for all windows). write is an
// optional legacy SSE sink: the direct /chat/stream handler passes one, the queue
// worker passes nil (the hub carries the UI either way). clientGone signals the
// submitting client navigated away so an interactive ask_user unblocks instead of
// pinning the detached turn; pass a never-done context for a server-driven queued
// turn that has no single owning client.
func (s *Server) runChatTurn(clientGone context.Context, wsp *workspace.Workspace, req chatReq, write func(event string, data any)) {
	// Register this turn so it can be stopped or steered while running.
	runID := uuid.NewString()
	// clientMsgID keys this turn's terminal hub events (turn_done / turn_error) so a
	// queue observer — the legacy /chat/stream + /chat handlers that now enqueue and
	// relay the hub — can tell its own turn's completion from another queued turn's.
	// Empty for the frontend's own /messages turns (harmless: it reads sessionTitle).
	clientMsgID := req.ClientMsgID
	// Detach the turn from the client connection so a page refresh/navigation never
	// cancels generation; only an explicit "stop" control cancels it.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(clientGone))
	defer cancel()
	// Inactivity watchdog for the interactive turn: a provider whose stream dies
	// silently (half-open socket, CLI subprocess that never exits) otherwise pins
	// this session forever and its queued inbox messages are never delivered. The
	// chat wrapper installs the heartbeat interval too, so a long step-less
	// operation is kept alive while a truly stalled stream is still reclaimed.
	ctx, stopTimeout := agent.WithChatActivityTimeout(runCtx, s.tun.ChatTurnTimeout(), s.chatTurnIdle())
	defer stopTimeout()
	run := s.runs.register(runID, req.SessionID, wsp.ID, cancel)
	defer s.runs.unregister(runID)
	// steer_undelivered fallback (Doc 59): a claude-cli steer is delivered at the
	// next tool boundary via the Interaction MCP permission tool's additionalContext.
	// If the turn ends (any path) with a steer message that never reached a tool
	// boundary (a tool-less, text-only turn), enqueue it as the next message so the
	// user's intent is not lost. takeSteer returns "" on a normal turn (or once the
	// message was already delivered), so this is a no-op in the common case.
	defer func() {
		if msg := run.takeSteer(); msg != "" {
			// To the FRONT of the queue: the user typed this steer to redirect THIS
			// turn, before anything they queued afterwards, so appending it to the tail
			// would deliver their oldest intent last.
			s.enqueueMessageFront(wsp.ID, chatReq{SessionID: req.SessionID, Message: msg, AgentIDs: req.AgentIDs})
		}
	}()
	ctx = agent.WithSteer(ctx, run.steer)
	// Point any CLI subprocess (claude-cli, ...) at the in-process Interaction MCP
	// endpoint for this turn, carrying the per-run token. No-op when unknown.
	if url := s.interactionURL(); url != "" {
		coreNames, extNames := splitInteractionTiers(interactionAdvertisedNames(s.tun, false), nil, nil)
		ctx = tools.WithInteractionEndpoint(ctx, url, run.token, coreNames, extNames)
	}
	// Install the legacy SSE writer only for the direct HTTP path; the queue worker
	// passes nil and run.emit becomes a no-op (the hub carries all UI).
	if write != nil {
		run.setWrite(write)
		defer run.clearWrite()
	}
	database := wsp.DB
	// The turn's phases (preflight → interactive wiring → lifecycle hooks →
	// stop-hook/agent passes → finish) run as methods on this value; it carries
	// the state they hand to each other, including the turn context they extend.
	t := &chatTurn{
		s:           s,
		wsp:         wsp,
		database:    database,
		req:         req,
		run:         run,
		runID:       runID,
		clientGone:  clientGone,
		clientMsgID: clientMsgID,
		sse:         run.emit,
		ctx:         ctx,
	}
	// Drop any crash-recovery sidecar when the turn returns by any normal path
	// (success, handled failure, client abort): only a true mid-turn process
	// death must leave it behind for the next boot to reclaim.
	defer database.ClearInflight(req.SessionID)

	// Ensure a terminal chat event fires even when generation fails after the
	// turn has begun: the frontend uses it to clear the post-reload "thinking"
	// indicator and reload the transcript. The success path sets emitted=true
	// and publishes its own richer event below.
	defer func() {
		if !t.started || t.emitted {
			return
		}
		wsp.Runtime.Emit(events.Event{
			Type:   "chat",
			Level:  "error",
			Title:  "Sohbet turu sonlandı",
			Target: map[string]string{"view": "chat", "sessionId": req.SessionID},
		})
	}()

	// Phase 1: load the session, claim the per-session turn slot, resolve the
	// responding agents and persist the user message. The slot is claimed inside,
	// so its release is deferred here even when the phase then fails.
	release, ok := t.preflight()
	if release != nil {
		defer release()
	}
	if !ok {
		return
	}
	// Phase 2: interactive prompts (ask_user, permissions) + session grants.
	t.wireInteractive()
	// Phase 3: the once-per-turn lifecycle hooks; a UserPromptSubmit veto ends the
	// turn here without calling any model.
	lifecycleContext, stop := t.runPromptLifecycle()
	if stop {
		return
	}
	// Phase 4: the agents answer, re-run while a Stop hook keeps blocking.
	if !t.runStopPasses(lifecycleContext) {
		return
	}
	// Phase 5: auto-title + the terminal success events.
	t.finishTurn()
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
func (s *Server) failTurn(ctx context.Context, wsp *workspace.Workspace, sse func(string, any), sessionID, agentID, clientMsgID, reason, detail string) {
	database := wsp.DB
	// Surface the failure in the server log too — without this a turn that dies
	// before producing output (provider unavailable, compaction failure, …) is
	// invisible server-side and only visible as a red card in the UI.
	s.logger.Error("turn failed", "session", sessionID, "agent", agentID, "reason", reason, "detail", detail)
	step := agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason}
	payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": clientMsgID}
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
	// Terminal error onto the hub too, so every window (and a queue observer waiting
	// on this turn) clears its "thinking" state and sees the failure — previously
	// failTurn only wrote the legacy SSE sink, leaving hub clients to discover it on
	// reload and the /chat + /chat/stream queue observers hanging.
	s.publishHub(wsp.ID, sessionID, sessionhub.KindTurnError, payload, false)
	s.hub.Commit(wsp.ID, sessionID)
}

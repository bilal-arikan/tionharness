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
	s.runChatTurnCaptured(clientGone, wsp, req, write, nil)
}

// runChatTurnCaptured exposes the registered run to the queue's outer panic
// barrier. The callback retains only generation identity; it must not do I/O.
func (s *Server) runChatTurnCaptured(clientGone context.Context, wsp *workspace.Workspace, req chatReq, write func(event string, data any), registered func(*chatRun)) {
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
	ctx, stopTimeout := agent.WithChatActivityTimeout(runCtx, 0, s.chatTurnIdle())
	defer stopTimeout()
	run := s.runs.register(runID, req.SessionID, wsp.ID, cancel)
	defer s.runs.unregister(runID)
	if registered != nil {
		registered(run)
	}
	run.setActivityTracker(agent.ActivityTrackerFrom(ctx), s.chatTurnIdle())
	// steer_undelivered fallback (Doc 59): guidance that never reached the model is
	// enqueued as the next message instead of dying with the run.
	defer func() { s.recoverUndeliveredSteer(run, wsp.ID, req) }()
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
	defer t.clearInflight()

	// Ensure a terminal chat event fires even when generation fails after the
	// turn has begun: the frontend uses it to clear the post-reload "thinking"
	// indicator and reload the transcript. The success path sets emitted=true
	// and publishes its own richer event below.
	defer func() {
		if !t.started || t.emitted {
			return
		}
		releaseGeneration, current := s.runs.acquireCurrent(run)
		if !current {
			return
		}
		defer releaseGeneration()
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
func (t *chatTurn) failTurn(agentID, reason, detail string) {
	s := t.s
	sessionID := t.req.SessionID
	// Surface the failure in the server log too — without this a turn that dies
	// before producing output (provider unavailable, compaction failure, …) is
	// invisible server-side and only visible as a red card in the UI.
	s.logger.Error("turn failed", "session", sessionID, "agent", agentID, "reason", reason, "detail", detail)
	step := agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason}
	payload := map[string]any{"error": detail, "reason": reason, "clientMsgId": t.clientMsgID}
	t.withGeneration(func() {
		if msg, err := t.database.AddMessage(t.ctx, db.Message{
			SessionID: sessionID,
			Role:      providers.RoleAssistant,
			AgentID:   agentID,
			Steps:     marshalSteps([]agent.TurnStep{step}),
		}); err != nil {
			s.logger.Error("persist turn error failed", "session", sessionID, "error", err)
		} else {
			payload["replyMessage"] = msg
		}
		t.sse("error", payload)
		// Terminal error reaches every window and queue observer atomically with
		// the durable error under the same generation fence.
		s.publishHub(t.wsp.ID, sessionID, sessionhub.KindTurnError, payload, false)
		s.hub.Commit(t.wsp.ID, sessionID)
	})
}

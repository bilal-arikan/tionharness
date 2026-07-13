package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/conversation"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// withTurnID mirrors withSessionID (workdir_context.go): a package-name-safe
// wrapper for handleChat, whose local `agent` variable shadows the agent package.
func withTurnID(ctx context.Context, id string) context.Context {
	return agent.WithTurnID(ctx, id)
}

// inflightRecorder accumulates a NON-STREAMING turn's persistable trace and
// snapshots it to the session's inflight sidecar on a throttle — crash-recovery
// parity with the streaming path (chat_stream.go `snapshot`). Previously only
// the SSE path wrote the sidecar, so a mid-turn process death (a dev rebuild, a
// crash) silently lost the whole non-stream turn: reply, trace, usage (the
// AlgoBench v2 incident). Steps arrive from the calling goroutine
// (CompleteWithToolsStream contract), so no locking is needed.
type inflightRecorder struct {
	db        *db.DB
	sessionID string
	agentID   string
	replyID   string
	startedAt int64
	partial   strings.Builder
	kept      []agent.TurnStep
	lastSnap  time.Time
}

func (rec *inflightRecorder) onStep(st agent.TurnStep) {
	switch st.Kind {
	case agent.StepDelta:
		rec.partial.WriteString(st.Text)
	case agent.StepAsk, agent.StepToolDelta, agent.StepTombstone, agent.StepPermission:
		// Transient (live-UI only) — never part of the persisted trace.
	default:
		rec.kept = append(rec.kept, st)
	}
	if time.Since(rec.lastSnap) < 600*time.Millisecond {
		return
	}
	rec.lastSnap = time.Now()
	_ = rec.db.WriteInflight(db.InflightTurn{
		MessageID: rec.replyID,
		SessionID: rec.sessionID,
		AgentID:   rec.agentID,
		StartedAt: rec.startedAt,
		Text:      rec.partial.String(),
		Steps:     marshalSteps(rec.kept),
	})
}

// interruptedTrace returns the steps kept so far with a trailing error step —
// the streaming path's "preserve what the agent already produced" shape.
func (rec *inflightRecorder) interruptedTrace(detail, reason string) []agent.TurnStep {
	return append(rec.kept, agent.TurnStep{Kind: agent.StepError, Text: detail, Reason: reason})
}

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
	// ClientMsgID is a client-generated id (ULID) for the send-queue path
	// (POST /sessions/{id}/messages): it dedupes double-submits / retries /
	// reconnect replays so the same message is enqueued at most once. Empty on the
	// legacy direct /chat/stream path. See _Docs/58-QUEUE-SENKRON.md Faz 3.
	ClientMsgID string `json:"clientMsgId,omitempty"`
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
	// Rendered as a volatile dynamic block — NOT folded into the history — so the
	// history messages stay byte-stable for the rolling prompt-cache breakpoint.
	toolRecap := recentToolActivityBlock(history)
	// Carry this workspace's editable compaction prompt onto the turn context so
	// any fold (rolling summary here, or reactive mid-loop downstream) uses it.
	ctx = conversation.WithCompactPrompt(ctx, ws(r).Runtime.CompactPromptTemplate())
	prep, err := s.convo.Prepare(ctx, database, provider, session, agent, history)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "compaction failed: "+err.Error())
		return
	}

	// Blocking (non-streaming) path: lifecycle hooks fire on the streaming path
	// (the UI default); "" here satisfies the request builder signature.
	llmReq := s.composeTurnRequest(ctx, ws(r), session, agent, []db.Agent{agent}, req.Message, prep, freshSession, multiAgent, toolRecap, "")
	// Prompt-epoch drift step (streaming-path parity): prepend a context_change
	// step to the persisted trace once per drift episode so the change is visible
	// in history. The agent already read the diff via the suffix note above. Type
	// inference keeps this free of the agent package name, which the db.Agent
	// variable `agent` shadows in this handler.
	leadSteps := consumeContextChangeLead(ws(r).Runtime, session.ID, agent.ID)

	// Manual chat is not budget-gated (autonomous=false). When the agent has
	// tools enabled this drives the agentic loop (native) or CLI delegation;
	// usage is recorded inside CompleteWithTools. Attach an artifact sink so
	// create_artifact / update_artifact can persist content this turn.
	ctx = tools.WithGrants(ctx, s.grants.forSession(session.ID))
	ctx = tools.WithArtifacts(ctx, newArtifactSink(database, session.ID, agent.ID, ws(r).Runtime.Emit))
	ctx = withSessionID(ctx, session.ID) // resolve this session's WorkingDir downstream
	// Crash-recovery parity with the streaming path: pre-allocate the reply id
	// (debug events tag it; sidecar and final message share one identity, so boot
	// recovery is idempotent) and snapshot the in-flight turn to the sidecar on a
	// throttle. A mid-turn process death then leaves the partial turn for boot to
	// reclaim instead of silently losing it.
	replyID := uuid.NewString()
	ctx = withTurnID(ctx, replyID)
	rec := &inflightRecorder{db: database, sessionID: session.ID, agentID: agent.ID, replyID: replyID, startedAt: start.Unix()}
	resp, steps, err := ws(r).Runtime.CompleteWithToolsStream(ctx, agent, provider, llmReq, false, rec.onStep)
	if err != nil {
		s.logger.Error("provider completion failed", "error", err, "agent", agent.ID)
		// Streaming-path parity: persist the partial trace as an interrupted
		// message so the tools/text the agent already produced stay visible in the
		// transcript, then drop the sidecar — the failure is handled, there is
		// nothing left for boot to recover. The HTTP contract is unchanged (502).
		persistCtx := context.WithoutCancel(ctx)
		trace := rec.interruptedTrace("provider error: "+err.Error(), "provider_error")
		if _, aerr := database.AddMessage(persistCtx, db.Message{
			ID:          replyID,
			SessionID:   session.ID,
			Role:        providers.RoleAssistant,
			AgentID:     agent.ID,
			Text:        rec.partial.String(),
			Steps:       marshalSteps(trace),
			Interrupted: true,
			DurationMs:  time.Since(start).Milliseconds(),
		}); aerr != nil {
			s.logger.Error("persist interrupted turn failed", "session", session.ID, "error", aerr)
		} else {
			// Keep any files written before the failure as artifacts (when the
			// workspace opts into auto-capture; otherwise only deliberate
			// create_artifact calls produce artifacts).
			if ws(r).Settings().AutoCaptureArtifacts {
				s.captureFileArtifacts(persistCtx, database, session.ID, agent.ID, trace)
			}
		}
		_ = database.ClearInflight(session.ID)
		// Self-healing parity: a failed turn must still auto-tag — error/stuck
		// counters, lesson reflection, failed-turn automations all hang off
		// AutoTagTurn. External clients drive this non-SSE endpoint (Doc 33), so
		// it cannot be left out of the loop.
		ws(r).Runtime.AutoTagTurn(persistCtx, session.ID, trace, "provider_error: "+err.Error())
		writeError(w, http.StatusBadGateway, "provider error: "+err.Error())
		return
	}

	// Persist the assistant reply (with its serialised activity trace so the
	// turn can be re-rendered on reload).
	replyMsg, err := database.AddMessage(ctx, db.Message{
		ID:         replyID,
		SessionID:  session.ID,
		Role:       providers.RoleAssistant,
		AgentID:    agent.ID,
		Text:       resp.Text,
		Steps:      marshalSteps(append(leadSteps, steps...)),
		Model:      resp.Model,
		StopReason: resp.StopReason,
		Usage:      messageUsage(resp.Usage),
		DurationMs: time.Since(start).Milliseconds(),
	})
	if err != nil {
		// Deliberately NOT clearing the sidecar: the reply exists only in memory
		// now, so the orphaned snapshot is the recovery net for the next boot.
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Reply is durable — drop the crash sidecar.
	_ = database.ClearInflight(session.ID)

	// Auto-capture any files the agent wrote this turn as artifacts (workspace
	// opt-in; off = only deliberate create_artifact calls register artifacts).
	if ws(r).Settings().AutoCaptureArtifacts {
		s.captureFileArtifacts(ctx, database, session.ID, agent.ID, steps)
	}

	// Streaming-path parity: auto-tag the finished turn (tool-error tags, stuck
	// counter reset, lesson reflection) and signal tag automations — previously
	// only the SSE path did this, silently exempting external non-SSE clients
	// from the whole self-healing loop.
	ws(r).Runtime.AutoTagTurn(ctx, session.ID, steps, "")
	ws(r).Runtime.FireTurnFinished(session.ID, agent.ID, resp.Text)

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

// buildSystemPrompt delegates to the agent package's single persona assembler
// (soul + identity + goal-usage hint) so the chat/preview and headless paths can
// never drift apart.
func buildSystemPrompt(a db.Agent) string {
	return agent.BuildSystemPrompt(a)
}

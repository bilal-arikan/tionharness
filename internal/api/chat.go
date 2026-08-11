package api

import (
	"context"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/providers"
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
	// turnSlotHeld marks a turn whose per-session runtime turn slot was ALREADY
	// claimed by the caller — the send-queue worker, which now claims it before
	// popping the head so a queued message stays visible (and cancellable) in the
	// queue tray while it waits, instead of vanishing into a blocking claim inside
	// the turn. Unexported on purpose: it is an internal hand-off, never client
	// input (JSON decoding cannot set it). See inbox.go / _Docs/58.
	turnSlotHeld bool
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

// handleChat is defined in chat_queue.go: after the durable cutover (_Docs/58) it
// enqueues onto the session's serial send-queue and returns the persisted reply,
// rather than running the turn inline. The inline provider path that used to live
// here was removed with that cutover; inflightRecorder (below) is retained because
// the streaming worker path and its test still exercise it.

// buildSystemPrompt delegates to the agent package's single persona assembler
// (soul + identity) so the chat/preview and headless paths can
// never drift apart.
func buildSystemPrompt(a db.Agent) string {
	return agent.BuildSystemPrompt(a)
}

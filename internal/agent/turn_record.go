package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// recordChildAssistantMessage is the single persistence seam for delegated
// session transcripts. It redacts tool arguments before writing and only marks
// the child successful after that write commits. A transcript failure therefore
// cannot leave a child looking completed.
func (r *Runtime) recordChildAssistantMessage(ctx context.Context, sessionID, agentID, text string, steps []TurnStep, meta *turnMeta, elapsedMs int64, terminalState string) error {
	if err := r.recordAssistantMessage(ctx, sessionID, agentID, text, childTranscriptSteps(steps), meta, elapsedMs); err != nil {
		stateErr := r.db.SetSessionRunState(ctx, sessionID, turnStatusFailed, time.Now().Unix())
		if stateErr != nil {
			return fmt.Errorf("persist child transcript: %w (persist failed run state: %v)", err, stateErr)
		}
		return fmt.Errorf("persist child transcript: %w", err)
	}
	if err := r.db.SetSessionRunState(ctx, sessionID, terminalState, time.Now().Unix()); err != nil {
		return fmt.Errorf("persist child %s run state: %w", terminalState, err)
	}
	return nil
}

// This file holds the shared reply/error recording for session-reuse continuation
// turns — a turn delivered into an EXISTING session (scheduled prompt, wake, and
// future adopters like the inbox worker / coordinator auto-turns). Only the
// most-duplicated, drift-prone block is shared: constructing the assistant
// message, substituting an empty reply, encoding the step trace, and stamping
// turn meta. The surrounding lifecycle (slot claim, ctx flags, which invoke to
// call, events, auto-continue/handoff, tag-fired automations) stays in each
// caller where it is readable — deliberately NOT folded into one parameterized
// runner, which would need a dozen knobs and read worse than the callers do.

// recordAssistantMessage is the shared tail of every continuation turn: it builds
// the assistant message (agent id + encoded step trace), stamps turn meta
// (model / usage / duration), and persists it. It performs NO empty-substitution
// — callers that need it (or that pre-compose stop/fail/empty text, like the
// coordinator/worker turns) shape `text` first. Returns the AddMessage error; the
// caller decides whether to propagate or just log it (each site keeps its own
// distinct warning message).
func (r *Runtime) recordAssistantMessage(ctx context.Context, sessionID, agentID, text string, steps []TurnStep, meta *turnMeta, elapsedMs int64) error {
	msg := db.Message{
		SessionID: sessionID,
		AgentID:   agentID,
		Role:      "assistant",
		Text:      text,
		Steps:     encodeSteps(steps),
	}
	meta.apply(&msg, elapsedMs)
	_, err := r.db.AddMessage(ctx, msg)
	return err
}

// recordAssistantReply persists an agent's reply as an assistant turn, replacing
// an empty reply with emptyText first (so the thread never reads as a silent
// no-reply). Returns the persisted text (after substitution, which callers reuse
// for tag-fired automations) and the AddMessage error.
func (r *Runtime) recordAssistantReply(ctx context.Context, sessionID, agentID, output string, steps []TurnStep, meta *turnMeta, elapsedMs int64, emptyText string) (string, error) {
	if strings.TrimSpace(output) == "" {
		output = emptyText
	}
	return output, r.recordAssistantMessage(ctx, sessionID, agentID, output, steps, meta, elapsedMs)
}

// recordTurnError persists a failed turn as an assistant message so the failure
// reads inline in the session thread (not only in logs/notifications). prefix is
// the human lead-in (e.g. "⚠️ Zamanlanmış prompt çalıştırılamadı:"); the error
// text follows on its own paragraph. Best-effort: an AddMessage failure is
// logged, not returned — the caller already has the real invoke error to
// propagate and tag on.
func (r *Runtime) recordTurnError(ctx context.Context, sessionID, agentID string, invokeErr error, steps []TurnStep, meta *turnMeta, elapsedMs int64, prefix string) {
	if err := r.recordAssistantMessage(ctx, sessionID, agentID, prefix+"\n\n"+invokeErr.Error(), steps, meta, elapsedMs); err != nil {
		r.logger.Warn("record turn error reply failed", "session", sessionID, "error", err)
	}
}

package agent

import (
	"context"
	"encoding/json"

	"github.com/bilal-arikan/swarmgo/internal/events"
)

// busForwardable reports whether a live TurnStep should be broadcast on the
// process-wide event bus for session-step subscribers (other windows viewing the
// session, or an autonomous turn with no per-request SSE of its own).
//
// The high-frequency token/output chunks (delta / tool_delta) are dropped — they
// would flood the shared bus and the full text/output lands on the persisted
// message anyway. The interactive prompts (ask / permission / plan) are dropped
// too: only the window that OWNS the running turn holds the runId needed to
// answer them, so surfacing them to a passive observer would be a dead card.
// Everything else — thinking, tool calls/results, todos, diffs, recovery,
// errors, subagents — is meaningful activity worth showing live.
func busForwardable(k StepKind) bool {
	switch k {
	case StepDelta, StepToolDelta, StepAsk, StepPermission, StepPlan, StepTombstone:
		return false
	}
	return true
}

// emitSessionStep broadcasts one turn step to the process-wide event bus tagged
// with its session id, so any window live-viewing that session — or an autonomous
// turn (scheduler/spawn/worker/wake) that has no per-request SSE — can render the
// activity as it happens. Best-effort: the authoritative trace is always
// persisted on the finished assistant message, so a dropped step is only a
// missed live frame, never lost history.
func (r *Runtime) emitSessionStep(sessionID string, st TurnStep) {
	if sessionID == "" || !busForwardable(st.Kind) {
		return
	}
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   "session_step",
		Level:  "info",
		Target: map[string]string{"sessionId": sessionID},
		Step:   b,
	})
}

// EmitSessionStep is the exported form of emitSessionStep for callers outside
// the agent package (the chat stream handler) that already hold the session id:
// it mirrors an interactive turn's live steps onto the bus so OTHER windows
// viewing the same session render it live too (the originating window suppresses
// the duplicate — it already streams over its own per-request SSE).
func (r *Runtime) EmitSessionStep(sessionID string, st TurnStep) { r.emitSessionStep(sessionID, st) }

// SessionStepEmitter returns an onStep sink that broadcasts the running turn's
// steps to the session bus, or nil when the turn carries no session id (so the
// caller keeps the plain non-streaming path). Autonomous turn helpers
// (invokeTraced / the wake-turn runner) pass this to CompleteWithToolsStream so
// scheduler/spawn/worker/wake/peer turns get the same live activity feed an
// interactive chat turn has.
func (r *Runtime) SessionStepEmitter(ctx context.Context) func(TurnStep) {
	sid := SessionIDFrom(ctx)
	if sid == "" {
		return nil
	}
	return func(st TurnStep) { r.emitSessionStep(sid, st) }
}

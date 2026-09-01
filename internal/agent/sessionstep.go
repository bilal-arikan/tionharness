package agent

import (
	"context"
	"encoding/json"

	"github.com/bilal-arikan/tionharness/internal/events"
)

// busForwardable reports whether a live TurnStep should be broadcast on the
// process-wide event bus for session-step subscribers (other windows viewing the
// session, or an autonomous turn with no per-request SSE of its own).
//
// Ephemeral live frames (Running / Append) are dropped — they would either flood
// the shared bus or persist a card that can never finish after session reload;
// the final replacement lands on the persisted message. The interactive prompts
// (ask / permission / plan) are dropped
// too: only the window that OWNS the running turn holds the runId needed to
// answer them, so surfacing them to a passive observer would be a dead card.
// Everything else — thinking, tool calls/results, todos, diffs, recovery,
// errors, subagents — is meaningful activity worth showing live.
func busForwardable(st TurnStep) bool {
	if st.Append || st.Running {
		return false
	}
	switch st.Kind {
	case StepDelta, StepAsk, StepPermission, StepPlan, StepTombstone:
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
func (r *Runtime) emitSessionStep(sessionID string, st TurnStep, origin string) {
	if sessionID == "" || !busForwardable(st) {
		return
	}
	target := map[string]string{"sessionId": sessionID}
	// origin lets the hub bridge tell an INTERACTIVE turn's mirrored step (which
	// the chat handler already published to the hub in-order) from an AUTONOMOUS
	// turn's step (which only reaches the hub via that bridge). Interactive steps
	// must never be re-bridged: a straggler processed AFTER the run unregisters
	// would re-arm the client's live indicator that turn_done just cleared (the
	// stuck-"conversing" bug). Empty origin = autonomous (bridged as before).
	if origin != "" {
		target["origin"] = origin
	}
	r.publishStep(events.TypeSessionStep, target, st)
}

// publishStep is the shared tail of both live-step emitters — the chat/autonomous
// session feed (session_step) and the flow run inspector (flow_node_step). Both
// marshal one TurnStep and broadcast it on the process-wide bus under a distinct
// event type + addressing target; routing this through one seam keeps the live-step
// envelope (Level/Step field) from drifting between the two feeds. The forwardable
// filter stays in each caller: the session feed drops high-frequency delta frames
// (busForwardable) while a flow node's captured steps are already coarse-grained.
func (r *Runtime) publishStep(evType string, target map[string]string, st TurnStep) {
	b, err := json.Marshal(st)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   evType,
		Level:  "info",
		Target: target,
		Step:   b,
	})
}

// EmitSessionStep is the exported form of emitSessionStep for callers outside
// the agent package (the chat stream handler) that already hold the session id:
// it mirrors an interactive turn's live steps onto the bus so OTHER windows
// viewing the same session render it live too (the originating window suppresses
// the duplicate — it already streams over its own per-request SSE).
func (r *Runtime) EmitSessionStep(sessionID string, st TurnStep) {
	r.emitSessionStep(sessionID, st, "interactive")
}

// SessionStepEmitter returns an onStep sink that broadcasts the running turn's
// steps to the session bus, or nil when the turn carries no session id (so the
// caller keeps the plain non-streaming path). Autonomous turn helpers
// (invokeTraced / the wake-turn runner) pass this to CompleteWithToolsStream so
// scheduler/spawn/worker/wake/peer turns get the same live activity feed an
// interactive chat turn has.
func (r *Runtime) SessionStepEmitter(ctx context.Context) func(TurnStep) {
	sid := SessionIDFrom(ctx)
	tracker := ActivityTrackerFrom(ctx)
	if sid == "" && tracker == nil {
		return nil
	}
	return func(st TurnStep) {
		if tracker != nil {
			tracker.ObserveStep(st)
		}
		if sid != "" {
			r.emitSessionStep(sid, st, "")
		}
	}
}

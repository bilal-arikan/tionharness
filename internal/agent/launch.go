package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// RunTrigger names what initiated a run (attribution/telemetry). The cross-cutting
// launch decision (which driver, precondition checks, flow-failure normalization)
// lives in LaunchRun; each launcher keeps its own guards (autonomy pause, cooldown,
// iteration caps, budget) and post-dispatch bookkeeping (records, notifications).
type RunTrigger string

const (
	TriggerManual          RunTrigger = "manual"
	TriggerSchedule        RunTrigger = "schedule"
	TriggerAutomationTag   RunTrigger = "automation:tag"
	TriggerAutomationBoard RunTrigger = "automation:board"
)

// RunSpec describes a run to launch by exactly one driver: a flow (FlowID set,
// takes precedence) or a spawned session (AgentID + Spawn). Trigger records the
// initiator for logs/telemetry.
type RunSpec struct {
	Trigger    RunTrigger
	Input      string
	Autonomous bool
	// flow driver:
	FlowID string
	// session driver:
	AgentID string
	Spawn   SpawnOptions
}

// LaunchResult is the unified outcome of a launch: which driver ran and the
// session it produced (a flow run also records into its own per-run transcript
// session, so both drivers yield a SessionID).
type LaunchResult struct {
	Driver    string     // "flow" | "session"
	SessionID string     // the produced/target session
	FlowRun   db.FlowRun // zero value for the session driver
}

// LaunchRun is the single dispatch for a triggered run: it validates the target,
// routes to the flow engine (RunFlowRecorded) or a spawned session (SpawnSession)
// per the spec, and normalizes a flow-failure into an error so every caller
// reports it identically. This consolidates the driver decision that was
// duplicated across the scheduler and automation launchers; callers keep their own
// pre-dispatch guards and post-dispatch records/notifications.
//
// It is the unified-Run (Model C) launcher seam: a future budget/pause/telemetry
// hook applies here once, for every trigger. Session-reuse launchers (the
// scheduler's schedule-kind prompt turn, schedule_wake into an existing session)
// are intentionally NOT folded into LaunchRun — they deliver a turn into an
// EXISTING session rather than spawning a fresh one, a distinct lifecycle (slot
// claim, history-aware invoke, auto-continue/handoff, wake events). Their shared,
// drift-prone part — building the reply/error message with meta + trace — is
// extracted into Runtime.recordAssistantReply / recordTurnError (turn_record.go);
// the lifecycle itself stays per-caller by design (folding it would need a
// dozen-knob runner that reads worse than the callers).
func (r *Runtime) LaunchRun(ctx context.Context, spec RunSpec) (LaunchResult, error) {
	if spec.FlowID != "" {
		if _, err := r.db.GetFlow(ctx, spec.FlowID); err != nil {
			return LaunchResult{Driver: "flow"}, fmt.Errorf("target flow gone: %w", err)
		}
		run, sessionID, err := r.RunFlowRecorded(ctx, spec.FlowID, spec.Input, spec.Autonomous, nil)
		if err != nil {
			return LaunchResult{Driver: "flow", SessionID: sessionID}, fmt.Errorf("flow run failed: %w", err)
		}
		if run.Status == db.FlowFailure {
			return LaunchResult{Driver: "flow", SessionID: sessionID, FlowRun: run}, fmt.Errorf("flow run failed: %s", run.Error)
		}
		return LaunchResult{Driver: "flow", SessionID: sessionID, FlowRun: run}, nil
	}

	if _, err := r.db.GetAgent(ctx, spec.AgentID); err != nil {
		return LaunchResult{Driver: "session"}, fmt.Errorf("target agent gone: %w", err)
	}
	res, err := r.SpawnSession(ctx, spec.AgentID, spec.Input, spec.Spawn)
	if err != nil {
		return LaunchResult{Driver: "session"}, fmt.Errorf("spawn failed: %w", err)
	}
	return LaunchResult{Driver: "session", SessionID: res.SessionID}, nil
}

package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// RunTrigger names what initiated a run (attribution/telemetry). The cross-cutting
// launch decision (which driver, precondition checks, flow-failure normalization)
// lives in LaunchRun; each launcher keeps its own guards (autonomy pause, cooldown,
// iteration caps, budget) and post-dispatch bookkeeping (records, notifications).
type RunTrigger string

const (
	TriggerManual            RunTrigger = "manual"
	TriggerSchedule          RunTrigger = "schedule"
	TriggerAutomationTag     RunTrigger = "automation:tag"
	TriggerAutomationBoard   RunTrigger = "automation:board"
	TriggerAutomationToken   RunTrigger = "automation:token"
	TriggerAutomationCounter RunTrigger = "automation:counter"
)

// RunSpec describes a run to launch by exactly one driver: a flow (FlowID set,
// takes precedence) or a spawned session (AgentID + Spawn). Trigger records the
// initiator for logs/telemetry.
type RunSpec struct {
	Trigger        RunTrigger
	Input          string
	Autonomous     bool
	IdempotencyKey string
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
	driver := launchDriver(spec)
	if err := r.launchGate(spec, driver); err != nil {
		return LaunchResult{Driver: driver}, err
	}
	if spec.FlowID != "" {
		if spec.IdempotencyKey != "" {
			if session, err := r.db.GetSessionByDispatchKey(ctx, spec.IdempotencyKey); err == nil {
				run, _ := r.db.GetFlowRunByDispatchKey(ctx, spec.IdempotencyKey)
				return LaunchResult{Driver: "flow", SessionID: session.ID, FlowRun: run}, nil
			}
		}
		if _, err := r.db.GetFlow(ctx, spec.FlowID); err != nil {
			return LaunchResult{Driver: "flow"}, fmt.Errorf("target flow gone: %w", err)
		}
		run, sessionID, err := r.runFlowRecorded(ctx, spec.FlowID, spec.Input, spec.Autonomous, nil, spec.IdempotencyKey)
		if err != nil {
			return LaunchResult{Driver: "flow", SessionID: sessionID}, fmt.Errorf("flow run failed: %w", err)
		}
		if run.Status == db.FlowFailure {
			return LaunchResult{Driver: "flow", SessionID: sessionID, FlowRun: run}, fmt.Errorf("flow run failed: %s", run.Error)
		}
		return LaunchResult{Driver: "flow", SessionID: sessionID, FlowRun: run}, nil
	}

	if spec.IdempotencyKey != "" {
		if session, err := r.db.GetSessionByDispatchKey(ctx, spec.IdempotencyKey); err == nil {
			return LaunchResult{Driver: "session", SessionID: session.ID}, nil
		}
	}
	if _, err := r.db.GetAgent(ctx, spec.AgentID); err != nil {
		return LaunchResult{Driver: "session"}, fmt.Errorf("target agent gone: %w", err)
	}
	spec.Spawn.NoQueue = true
	spec.Spawn.IdempotencyKey = spec.IdempotencyKey
	res, err := r.SpawnSession(ctx, spec.AgentID, spec.Input, spec.Spawn)
	if err != nil {
		return LaunchResult{Driver: "session"}, fmt.Errorf("spawn failed: %w", err)
	}
	return LaunchResult{Driver: "session", SessionID: res.SessionID}, nil
}

// launchDriver reports which driver a spec routes to (flow takes precedence over
// the session driver), for gating and result labeling before the dispatch runs.
func launchDriver(spec RunSpec) string {
	if spec.FlowID != "" {
		return "flow"
	}
	return "session"
}

// launchGate is the single pre-dispatch gate every fresh triggered run passes
// through — the unified-Run (Model C) choke point the doc reserved on LaunchRun.
// It honors the workspace autonomy brake for autonomous launches BEFORE anything
// is spawned, so a paused workspace never creates a session / flow-run that would
// only fail at its first provider call (guardedComplete gates there too, but only
// after the junk row already exists). Manual launches bypass the brake. It also
// stamps one launch-telemetry line per run (trigger, driver, autonomous). Per-
// launcher guards (cooldown, iteration caps) still run in the caller beforehand.
func (r *Runtime) launchGate(spec RunSpec, driver string) error {
	if spec.Autonomous && r.Paused() {
		return ErrAutonomyPaused
	}
	r.logger.Info("launch",
		"trigger", spec.Trigger, "driver", driver, "autonomous", spec.Autonomous,
		"flow", spec.FlowID, "agent", spec.AgentID)
	return nil
}

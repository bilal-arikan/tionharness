package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// RunTrigger names what initiated a run (attribution/telemetry). The cross-cutting
// launch decision (precondition checks, idempotency) lives in LaunchRun; each launcher keeps its own guards (autonomy pause, cooldown,
// iteration caps, budget) and post-dispatch bookkeeping (records, notifications).
type RunTrigger string

const (
	TriggerManual          RunTrigger = "manual"
	TriggerSchedule        RunTrigger = "schedule"
	TriggerAutomationTag   RunTrigger = "automation:tag"
	TriggerAutomationBoard RunTrigger = "automation:board"
	// Rota (F2): a declared phase finished / was entered, a trajectory ended.
	TriggerAutomationPhase         RunTrigger = "automation:phase"
	TriggerAutomationTrajectoryEnd RunTrigger = "automation:trajectory_end"
	// Flows (_Docs/93): a flow run finished (flow-kind automation), or a
	// trigger node inside a run fired an automation of any kind.
	TriggerAutomationFlow RunTrigger = "automation:flow"
	TriggerFlowNode       RunTrigger = "flow:node"
)

// RunSpec describes a spawned-session run to launch. Trigger records the
// initiator for logs/telemetry.
type RunSpec struct {
	Trigger        RunTrigger
	Input          string
	Autonomous     bool
	IdempotencyKey string
	// Origin is the lineage record for the session this launch produces — who
	// started it (automation, schedule) and, for a session-scoped trigger, which
	// session tripped it (Spawn.Origin wins when the caller set both). Nil lets
	// the store derive a default from the session's own shape.
	Origin  *db.SessionOrigin
	AgentID string
	Spawn   SpawnOptions
}

// LaunchResult is the outcome of a launch: the session it produced.
type LaunchResult struct {
	Driver    string // always "session"
	SessionID string
}

// LaunchRun is the single dispatch for a triggered run: it validates the target
// agent and spawns a session (SpawnSession) per the spec, so every caller reports
// failures identically. The agent's own evolving flow shapes each turn of that
// session (_Docs/93); callers keep their own pre-dispatch guards and
// post-dispatch records/notifications.
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
	if spec.Spawn.Origin == nil {
		spec.Spawn.Origin = spec.Origin
	}
	res, err := r.SpawnSession(ctx, spec.AgentID, spec.Input, spec.Spawn)
	if err != nil {
		return LaunchResult{Driver: "session"}, fmt.Errorf("spawn failed: %w", err)
	}
	return LaunchResult{Driver: "session", SessionID: res.SessionID}, nil
}

// launchDriver reports the driver label a spec routes to, for gating and result
// labeling before the dispatch runs. Only the session driver remains.
func launchDriver(RunSpec) string { return "session" }

// launchGate is the single pre-dispatch gate every fresh triggered run passes
// through — the unified-Run (Model C) choke point the doc reserved on LaunchRun.
// It honors the workspace autonomy brake for autonomous launches BEFORE anything
// is spawned, so a paused workspace never creates a session that would
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
		"agent", spec.AgentID)
	return nil
}

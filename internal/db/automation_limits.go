package db

import (
	"errors"
	"fmt"
)

// Automation iteration limits.
//
// MaxIterations used to accept 0 as "unlimited", and no write path checked it.
// Because a card can be moved by the agent's own move_task call, a board-triggered
// automation could then loop forever with no human in the way. Three layers now
// stand between a user and that loop, and each covers what the others cannot:
//
//  1. ValidateMaxIterations at every ENTRY POINT — rejects <= 0 and > cap.
//  2. MaxIterationsHardCap — stops "effectively unlimited" from sneaking back in
//     as a large number once 0 is closed off.
//  3. AbsoluteIterationBackstop — for rows ALREADY on disk with <= 0, which never
//     pass through validation at all (written before this rule, imported from a
//     market package, or hand-edited JSON). Enforced at fire time.
//
// Validation alone would leave existing data unguarded; the backstop alone would
// let new unbounded automations keep being created.
const (
	// MaxIterationsHardCap is the largest lifetime trigger count a write may set.
	// Ten times the default (50): ample for any real configuration, while a
	// mistyped "1000000" still gets refused. Every iteration spawns a session and
	// spends real budget, so the ceiling is a cost guard, not a style rule.
	MaxIterationsHardCap = 500

	// AbsoluteIterationBackstop bounds a legacy automation stored with
	// MaxIterations <= 0. It is deliberately ABOVE MaxIterationsHardCap: this is a
	// last-resort brake for data that predates the rule, not a limit anyone chose,
	// so it should not stop a working setup earlier than an explicit maximum would.
	AbsoluteIterationBackstop = 1000

	// MinTokenThreshold is the smallest interval a token automation may set. A tiny
	// interval would cross on nearly every call and fire in a tight loop (bounded
	// only by cooldown/maxIterations); requiring at least this many tokens keeps a
	// token trigger a meaningful "spend milestone" rather than a per-call hook.
	MinTokenThreshold = 1000

	// MinCounterInterval is the smallest interval a counter automation may set. An
	// interval of 1 would fire on nearly every message/tool call; requiring at least
	// this many keeps a counter trigger a meaningful cadence ("every N messages")
	// rather than a per-append hook. Kept small because counters (unlike tokens)
	// grow slowly and predictably, so a modest floor is enough.
	MinCounterInterval = 2
)

// ErrCounterIntervalRange reports a counterInterval value below the accepted floor.
var ErrCounterIntervalRange = errors.New("counterInterval out of range")

// ValidateCounterInterval rejects a counter-automation interval that would fire
// too often to be useful. Shared by the REST handlers and the agent tools so the
// two entry points cannot drift apart.
func ValidateCounterInterval(v int) error {
	if v < MinCounterInterval {
		return fmt.Errorf("%w: en az %d olmalı (çok küçük bir aralık neredeyse her mesajda tetiklenir)",
			ErrCounterIntervalRange, MinCounterInterval)
	}
	return nil
}

// ErrTokenThresholdRange reports a tokenThreshold value below the accepted floor.
var ErrTokenThresholdRange = errors.New("tokenThreshold out of range")

// ValidateTokenThreshold rejects a token-automation interval that would fire too
// often to be useful. Shared by the REST handlers and the agent tools (like
// ValidateMaxIterations) so the two entry points cannot drift apart.
func ValidateTokenThreshold(v int) error {
	if v < MinTokenThreshold {
		return fmt.Errorf("%w: en az %d olmalı (çok küçük bir aralık her çağrıda tetiklenir)",
			ErrTokenThresholdRange, MinTokenThreshold)
	}
	return nil
}

// ErrMaxIterationsRange reports a maxIterations value outside the accepted range.
var ErrMaxIterationsRange = errors.New("maxIterations out of range")

// ValidateMaxIterations rejects an unbounded or absurd automation loop. It lives
// in db, not in one of the callers, so the REST handlers and the agent tools
// cannot drift apart — an agent calling create_automation must not be able to
// write a value the API would refuse.
// The message is wrapped with %w rather than errors.Join: Join separates its
// operands with a NEWLINE, which reached the UI as a two-line toast that also
// repeated the word "maxIterations" in both halves.
func ValidateMaxIterations(v int) error {
	switch {
	case v <= 0:
		return fmt.Errorf("%w: 1 ile %d arasında olmalı (0 = sınırsız artık kabul edilmiyor: sonsuz döngü riski)",
			ErrMaxIterationsRange, MaxIterationsHardCap)
	case v > MaxIterationsHardCap:
		return fmt.Errorf("%w: en fazla %d olabilir", ErrMaxIterationsRange, MaxIterationsHardCap)
	}
	return nil
}

// ErrAutomationShape reports a trigger/target combination that could never fire
// or has nothing to run.
var ErrAutomationShape = errors.New("invalid automation")

// ValidateAutomationShape enforces the per-trigger-kind requirements on a fully
// MERGED automation. It lives in db, next to ValidateMaxIterations, so every
// write path shares one contract: create_automation and update_automation cannot
// drift. Without it, update could persist a state create rejects — a tag rule
// with no triggerTag (silently never fires: TriggerTag=="" is skipped at fire
// time) or a spawn rule with no target (fails only at fire time). The per-field
// format checks (ValidBoardOp/ValidTokenScope) still run at the call sites for
// immediate feedback; this is the final backstop on the merged result. The RANGE
// validators (maxIterations, tokenThreshold) stay separate and are called
// alongside this one.
func ValidateAutomationShape(a Automation) error {
	switch a.TriggerKind {
	case TriggerBoard:
		if !ValidBoardOp(a.BoardOp) {
			return fmt.Errorf("%w: invalid boardOp %q (any|move|create|update|delete)", ErrAutomationShape, a.BoardOp)
		}
		if !ValidBoardAction(a.BoardAction) {
			return fmt.Errorf("%w: invalid boardAction %q (spawn|archive)", ErrAutomationShape, a.BoardAction)
		}
		// An archive action does bookkeeping with no LLM call, so it needs no target.
		if a.BoardAction == BoardActionArchive {
			return nil
		}
	case TriggerToken:
		if !ValidTokenScope(a.TokenScope) {
			return fmt.Errorf("%w: invalid tokenScope %q (session|workspace)", ErrAutomationShape, a.TokenScope)
		}
		if err := ValidateTokenThreshold(a.TokenThreshold); err != nil {
			return err
		}
	case TriggerCounter:
		if !ValidCounterMetric(a.CounterMetric) {
			return fmt.Errorf("%w: invalid counterMetric %q (message|tool)", ErrAutomationShape, a.CounterMetric)
		}
		if !ValidCounterScope(a.CounterScope) {
			return fmt.Errorf("%w: invalid counterScope %q (session|workspace)", ErrAutomationShape, a.CounterScope)
		}
		if err := ValidateCounterInterval(a.CounterInterval); err != nil {
			return err
		}
	default: // TriggerTag or "" (legacy files written before the kind existed)
		if a.TriggerTag == "" {
			return fmt.Errorf("%w: triggerTag is required for tag automations (an empty tag never fires)", ErrAutomationShape)
		}
	}
	// Session mode is a free choice across kinds, but the value must be known.
	if !ValidSessionMode(a.SessionMode) {
		return fmt.Errorf("%w: invalid sessionMode %q (spawn|continue)", ErrAutomationShape, a.SessionMode)
	}
	// Every automation that reaches here spawns a session or runs a flow, so it
	// needs exactly one runnable target. (Board 'archive' returned above.)
	if a.FlowID == "" && a.TargetAgentID == "" {
		return fmt.Errorf("%w: targetAgentId or flowId is required", ErrAutomationShape)
	}
	return nil
}

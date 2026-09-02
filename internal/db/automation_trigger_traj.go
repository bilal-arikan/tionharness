package db

import (
	"fmt"
	"strings"
)

// Trajectory ("Rota") trigger kinds (_Docs/77 F2).
//
// A phase automation fires when a declared phase of a trajectory changes state
// — by default when it EXITS (done / skipped / failed), optionally when it is
// ENTERED (becomes active). A trajectory_end automation fires when the whole
// trajectory reaches a terminal status. Both may narrow on the recipe the
// trajectory was seeded from; the phase kind may also narrow on one phase id.
// Recipe watchers (`phases[].watchers`, `watchers`) name automations by id or
// name and fire them on the same transitions without any filter — the rule
// itself may be of any kind, the recipe is the binding.
const (
	TriggerPhase         = "phase"
	TriggerTrajectoryEnd = "trajectory_end"
)

// Phase transition events a phase automation may watch.
const (
	TrajEventExit  = "exit"  // the phase finished: done / skipped / failed (default)
	TrajEventEnter = "enter" // the phase became active
)

// ValidTrajEvent reports whether ev is empty (exit) or a known event.
func ValidTrajEvent(ev string) bool {
	switch ev {
	case "", TrajEventExit, TrajEventEnter:
		return true
	}
	return false
}

// EffectiveTrajEvent resolves the empty event to exit.
func (a Automation) EffectiveTrajEvent() string {
	if a.TrajEvent == "" {
		return TrajEventExit
	}
	return a.TrajEvent
}

// ValidTrajStatus reports whether st is empty (any terminal status) or one of
// the terminal trajectory statuses a trajectory_end rule may narrow on.
func ValidTrajStatus(st string) bool {
	switch st {
	case "", TrajStatusDone, TrajStatusFailed, TrajStatusAbandoned:
		return true
	}
	return false
}

func init() {
	RegisterTrigger(TriggerSpec{
		Kind: TriggerPhase, Label: "Rota fazı",
		Validate: func(a Automation) error {
			if !ValidTrajEvent(a.TrajEvent) {
				return fmt.Errorf("%w: invalid trajEvent %q (exit|enter)", ErrAutomationShape, a.TrajEvent)
			}
			if p := strings.TrimSpace(a.TrajPhase); p != "" && strings.ContainsAny(p, " \t\n") {
				return fmt.Errorf("%w: trajPhase %q must be a single phase id", ErrAutomationShape, a.TrajPhase)
			}
			return nil
		},
	})
	RegisterTrigger(TriggerSpec{
		Kind: TriggerTrajectoryEnd, Label: "Rota sonu",
		Validate: func(a Automation) error {
			if !ValidTrajStatus(a.TrajStatus) {
				return fmt.Errorf("%w: invalid trajStatus %q (done|failed|abandoned or empty for any)", ErrAutomationShape, a.TrajStatus)
			}
			return nil
		},
	})
}

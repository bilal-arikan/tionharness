package db

import "encoding/json"

// ActivitySignal describes one message append's effect on a session's monotonic
// activity counters. It is delivered to the activity hook (see SetActivityHook)
// so counter-triggered automations can detect an interval crossing statelessly:
// the previous total is (NewTotal - Delta), mirroring the token path's use of a
// per-call delta. One signal carries BOTH metrics; a user message moves only the
// message counter (ToolDelta == 0), an assistant turn may move both.
type ActivitySignal struct {
	SessionID    string
	MessageTotal int // session lifetime message count AFTER this append
	MessageDelta int // messages this append added (always 1)
	ToolTotal    int // session lifetime tool-call count AFTER this append
	ToolDelta    int // tool calls this append's message carried (0 for non-tool msgs)
}

// ActivityFn observes a message-append activity signal.
type ActivityFn func(sig ActivitySignal)

// SetActivityHook registers (or clears, with nil) the activity observer. Wired
// once at workspace boot by the manager to the AutomationEngine.
func (d *DB) SetActivityHook(fn ActivityFn) {
	d.activityHookMu.Lock()
	d.activityHook = fn
	d.activityHookMu.Unlock()
}

// fireActivityHook dispatches an activity signal to the registered observer (if
// any). Called after the store lock is released.
func (d *DB) fireActivityHook(sig ActivitySignal) {
	d.activityHookMu.RLock()
	fn := d.activityHook
	d.activityHookMu.RUnlock()
	if fn != nil {
		fn(sig)
	}
}

// countToolSteps returns how many tool calls a persisted assistant message's
// Steps JSON carries — the steps whose kind is "tool" (agent.StepTool). It is a
// tolerant scan: an empty/"[]"/malformed Steps value counts as zero rather than
// erroring, because a bad transcript line must not break message persistence.
// The db layer cannot import agent.TurnStep (that would be an import cycle), so
// it decodes only the one field it needs.
func countToolSteps(stepsJSON string) int {
	if stepsJSON == "" || stepsJSON == "[]" {
		return 0
	}
	var steps []struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return 0
	}
	n := 0
	for _, s := range steps {
		if s.Kind == "tool" {
			n++
		}
	}
	return n
}

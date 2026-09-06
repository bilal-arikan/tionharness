package db

import (
	"encoding/json"
	"fmt"
)

// ActivitySignal describes one message append's effect on a session's monotonic
// activity counters. It is delivered to the activity hook (see SetActivityHook)
// as the "something happened in this session" signal the live event stream
// forwards to clients. The previous total is (NewTotal - Delta). One signal
// carries BOTH metrics; a user message moves only the message counter
// (ToolDelta == 0), an assistant turn may move both.
type ActivitySignal struct {
	EventID                  string // stable id for at-least-once durable delivery dedupe
	SessionID                string
	MessageTotal             int   // session lifetime message count AFTER this append
	MessageDelta             int   // messages this append added (always 1)
	ToolTotal                int   // session lifetime tool-call count AFTER this append
	ToolDelta                int   // tool calls this append's message carried (0 for non-tool msgs)
	WorkspaceMessageTotal    int64 // workspace total captured at this append
	WorkspaceToolTotal       int64 // workspace total captured at this append
	WorkspaceMessagePrevious int64 // workspace total immediately before this append
	WorkspaceToolPrevious    int64 // workspace total immediately before this append
}

// ActivityFn observes a message-append activity signal.
type ActivityFn func(sig ActivitySignal) error

// SetActivityHook registers (or clears, with nil) the activity observer. Wired
// once at workspace boot by the manager.
func (d *DB) SetActivityHook(fn ActivityFn) error {
	d.activityHookMu.Lock()
	d.activityHook = fn
	d.activityHookMu.Unlock()
	if fn == nil {
		return nil
	}
	if err := d.deliverPendingCLIReplyActivities(); err != nil {
		return fmt.Errorf("deliver pending CLI reply activity: %w", err)
	}
	return nil
}

func invokeActivityHook(fn ActivityFn, sig ActivitySignal) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("activity hook panic: %v", recovered)
		}
	}()
	return fn(sig)
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

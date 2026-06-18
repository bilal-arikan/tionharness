package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bilal/swarmgo/internal/providers"
)

// WakeFunc arms a one-shot self-wake: after delaySeconds the current agent is
// re-invoked with prompt inside the current chat session, so the conversation
// continues on its own. Supplied by the interactive chat layer; autonomous runs
// (and any turn with no originating session) leave it unset.
type WakeFunc func(ctx context.Context, delaySeconds int, prompt, reason string) (string, error)

// wakeKey keys the WakeFunc on a request context.
type wakeKey struct{}

// WithWakeScheduler attaches a wake scheduler to ctx so the schedule_wake tool
// can arm a self-wake mid-turn. Kept in the tools package (not agent) so built-in
// tools can reach it without importing the agent package (which would cycle).
func WithWakeScheduler(ctx context.Context, fn WakeFunc) context.Context {
	return context.WithValue(ctx, wakeKey{}, fn)
}

// wakeFrom returns the wake scheduler attached to ctx, or nil when none is
// present (autonomous runs, or turns with no originating chat session).
func wakeFrom(ctx context.Context) WakeFunc {
	fn, _ := ctx.Value(wakeKey{}).(WakeFunc)
	return fn
}

// ScheduleWakeTool lets an agent pause the conversation and have itself
// re-invoked after a delay — e.g. to wait for a long-running background job and
// then report on it, instead of dangling. The wake re-delivers a prompt into the
// same chat session as a fresh turn.
type ScheduleWakeTool struct{}

// NewScheduleWakeTool constructs schedule_wake.
func NewScheduleWakeTool() ScheduleWakeTool { return ScheduleWakeTool{} }

func (ScheduleWakeTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "schedule_wake",
		Description: "Pause this turn and have yourself automatically re-invoked after a delay to continue the conversation. Use this when you must WAIT for something (a long background job, a timer) before you can finish — instead of ending the turn while 'waiting'. When the delay elapses, `prompt` is delivered to you as a new turn in THIS chat session. delaySeconds is clamped to 5..3600. After calling this, stop — produce no further work this turn.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"delaySeconds":{"type":"integer","description":"How many seconds to wait before waking (5..3600)"},
				"prompt":{"type":"string","description":"The instruction delivered to you when you wake, e.g. 'Check the background scan results and report the largest folders'"},
				"reason":{"type":"string","description":"Short note on why you are waiting (shown to the user)"}
			},
			"required":["delaySeconds","prompt"],
			"additionalProperties":false
		}`),
	}
}

func (ScheduleWakeTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		DelaySeconds int    `json:"delaySeconds"`
		Prompt       string `json:"prompt"`
		Reason       string `json:"reason"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	fn := wakeFrom(ctx)
	if fn == nil {
		return "", fmt.Errorf("schedule_wake is only available during an interactive chat turn")
	}
	return fn(ctx, in.DelaySeconds, in.Prompt, in.Reason)
}

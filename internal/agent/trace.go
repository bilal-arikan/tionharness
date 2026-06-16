package agent

import (
	"encoding/json"

	"github.com/bilal/swarmgo/internal/providers"
)

// StepKind tags the kind of activity captured in a turn trace.
type StepKind string

const (
	// StepText is an intermediate assistant text segment emitted before the
	// final answer (e.g. the model narrating between tool calls).
	StepText StepKind = "text"
	// StepThinking is the model's reasoning/thinking content, when a provider
	// surfaces it.
	StepThinking StepKind = "thinking"
	// StepTool is a single tool invocation paired with its result.
	StepTool StepKind = "tool"
	// StepDelta is an incremental text chunk emitted while a streaming provider
	// produces the final answer token-by-token. Delta steps are transient (live
	// UI only) and are never persisted — the full text lands on the message.
	StepDelta StepKind = "delta"
	// StepAsk is a transient interactive prompt: the agent called the ask_user
	// tool and is blocked waiting for the user's answer. Like StepDelta it is
	// live-only (never persisted) — the resolved Q&A is persisted as the
	// ask_user tool step once the answer arrives.
	StepAsk StepKind = "ask"
)

// TurnStep is one entry in an assistant turn's activity trace. The ordered list
// of steps lets the chat UI re-render tool cards, thinking blocks and diffs
// exactly as they happened — even after a reload (the trace is persisted as
// JSON on the message). Only the fields relevant to Kind are populated.
type TurnStep struct {
	Kind StepKind `json:"kind"`
	// Text/Thinking payload.
	Text string `json:"text,omitempty"`
	// Tool payload.
	Tool    string          `json:"tool,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	Output  string          `json:"output,omitempty"`
	IsError bool            `json:"isError,omitempty"`
	// Options are the suggested clickable answers for a StepAsk prompt (optional;
	// the user may always type a free-text answer instead).
	Options []string `json:"options,omitempty"`
}

// traceStepToTurnStep maps a single provider trace step to an agent TurnStep.
func traceStepToTurnStep(t providers.TraceStep) TurnStep {
	return TurnStep{
		Kind:    StepKind(t.Kind),
		Text:    t.Text,
		Tool:    t.Tool,
		Input:   t.Input,
		Output:  t.Output,
		IsError: t.IsError,
	}
}

// traceToSteps converts a provider-produced trace (claude CLI stream-json) into
// the agent-level TurnStep records the chat UI renders.
func traceToSteps(tr []providers.TraceStep) []TurnStep {
	if len(tr) == 0 {
		return nil
	}
	steps := make([]TurnStep, 0, len(tr))
	for _, t := range tr {
		steps = append(steps, traceStepToTurnStep(t))
	}
	return steps
}

// encodeSteps serialises a trace to JSON for persistence. Returns "[]" for an
// empty/failed trace so the column is always valid JSON.
func encodeSteps(steps []TurnStep) string {
	if len(steps) == 0 {
		return "[]"
	}
	b, err := json.Marshal(steps)
	if err != nil {
		return "[]"
	}
	return string(b)
}

package agent

import (
	"encoding/json"
	"strings"

	"github.com/bilal/swarmgo/internal/providers"
)

// interactionToolPrefix is the MCP namespace the Interaction MCP server uses. The
// CLI reports its tools namespaced (mcp__swarmgo_interaction__ask_user); we strip
// it so the persisted trace shows the bare tool name and renders with the same
// cards as the native tool path (todo checklist, artifact card, ask).
const interactionToolPrefix = "mcp__swarmgo_interaction__"

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
	// StepTodo is the agent's working checklist (from the todo_write tool),
	// rendered as a first-class checklist card rather than a generic tool row.
	// The items live in TurnStep.Todos; persisted so the list survives reload.
	StepTodo StepKind = "todo"
	// StepRecovery marks the loop taking a non-happy-path branch (e.g. hitting
	// the tool-iteration cap). Reason carries a stable machine tag; Text is the
	// human-readable explanation. Persisted so the trace explains itself.
	StepRecovery StepKind = "recovery"
	// StepError is a turn-level failure surfaced inline (provider error, budget
	// exceeded, cancellation) — distinct from a tool's own error (StepTool with
	// IsError). Reason carries the machine tag, Text the message.
	StepError StepKind = "error"
	// StepSteer is live user guidance folded into a running turn (the steer
	// control). Text is the guidance. Rendered distinctly from model narration.
	StepSteer StepKind = "steer"
	// StepToolDelta is an incremental chunk of a long tool's output, streamed
	// live while the tool runs. Transient (live UI only, never persisted); chunks
	// sharing an ID belong to the same tool invocation and are concatenated.
	StepToolDelta StepKind = "tool_delta"
	// StepTombstone is a control signal (not rendered itself) telling the UI to
	// remove a previously emitted live step: Ref names the target step's ID. Used
	// to retract a stale/cancelled live step without resending the whole trace.
	StepTombstone StepKind = "tombstone"
	// StepPermission is a transient interactive approval prompt: a write/exec tool
	// is blocked under "ask" mode waiting for the user to approve or deny it. Like
	// StepAsk it is live-only (never persisted); the outcome surfaces as the tool
	// running (allowed) or a permission_denied StepError (denied). Tool names the
	// gated tool, Reason carries its risk tier, Options the answer choices.
	StepPermission StepKind = "permission"
	// StepDiff is a file mutation (write_file / edit_file) rendered as a diff card
	// — path plus added/removed line counts and an optional unified patch — rather
	// than a generic tool row. The payload lives in Path/Added/Removed/Patch.
	StepDiff StepKind = "diff"
)

// TodoItem is one entry in a StepTodo checklist (mirrors the todo_write input).
type TodoItem struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

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
	// Todos carries the checklist items for a StepTodo step.
	Todos []TodoItem `json:"todos,omitempty"`
	// Reason is the stable machine tag for a StepRecovery/StepError step (e.g.
	// "max_tool_iterations", "provider_error", "budget_exceeded").
	Reason string `json:"reason,omitempty"`
	// ID optionally identifies a live step so a later StepTombstone (or
	// StepToolDelta chunk) can reference it.
	ID string `json:"id,omitempty"`
	// Ref is the target step ID a StepTombstone retracts.
	Ref string `json:"ref,omitempty"`
	// StepDiff payload: the changed file path, its added/removed line counts, an
	// optional unified patch, and whether the file was newly created.
	Path    string `json:"path,omitempty"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
	Patch   string `json:"patch,omitempty"`
	Created bool   `json:"created,omitempty"`
}

// parseTodos extracts the checklist items from a todo_write tool call's input
// ({"todos":[{content,status}]}). Returns nil on any decode failure.
func parseTodos(input json.RawMessage) []TodoItem {
	var in struct {
		Todos []TodoItem `json:"todos"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return nil
	}
	return in.Todos
}

// traceStepToTurnStep maps a single provider trace step to an agent TurnStep.
// Interaction MCP tool calls (claude-cli path) are normalised to their bare names
// and a todo_write call is promoted to a first-class checklist step, matching the
// native tool loop so the CLI path renders the same cards.
func traceStepToTurnStep(t providers.TraceStep) TurnStep {
	tool := strings.TrimPrefix(t.Tool, interactionToolPrefix)
	st := TurnStep{
		Kind:    StepKind(t.Kind),
		Text:    t.Text,
		Tool:    tool,
		Input:   t.Input,
		Output:  t.Output,
		IsError: t.IsError,
	}
	if st.Kind == StepTool && !st.IsError && tool == "todo_write" {
		if todos := parseTodos(t.Input); len(todos) > 0 {
			st.Kind = StepTodo
			st.Todos = todos
		}
	}
	return st
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

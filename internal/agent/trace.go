package agent

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// interactionToolPrefix / extendedToolPrefix are the two MCP namespaces the
// Interaction MCP server uses (core eager tier vs extended deferred tier). The CLI
// reports its tools namespaced (mcp__tionswarm_interaction__ask_user,
// mcp__tionswarm_extended__create_agent); we strip either so the persisted trace shows
// the bare tool name and renders with the same cards as the native tool path (todo
// checklist, artifact card, ask).
const (
	interactionToolPrefix = "mcp__tionswarm_interaction__"
	extendedToolPrefix    = "mcp__tionswarm_extended__"
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
	// StepPlan is a transient interactive plan-approval prompt: a claude-cli agent
	// called ExitPlanMode to present its plan and is blocked waiting for the user to
	// approve or reject it. Like StepPermission it is live-only (never persisted);
	// the outcome surfaces as the turn proceeding (approved) or the model revising
	// (rejected, with the user's feedback). Text carries the plan markdown, Options
	// the answer choices. The CLI's stream-json trace persists the ExitPlanMode tool
	// call itself once the decision is made.
	StepPlan StepKind = "plan"
	// StepHook is an audit card for a user-defined PreToolUse/PostToolUse hook
	// firing around a tool call: Tool names the gated tool, Reason carries the
	// machine decision tag (hook_block / hook_modify / hook_allow / hook_context),
	// Text the human explanation and Output the hook's reason/detail. Persisted so
	// the trace shows why a call was blocked or its input/output rewritten.
	StepHook StepKind = "hook"
	// StepDiff is a file mutation (write_file / edit_file) rendered as a diff card
	// — path plus added/removed line counts and an optional unified patch — rather
	// than a generic tool row. The payload lives in Path/Added/Removed/Patch.
	StepDiff StepKind = "diff"
	// StepSubagent is one run_subagent invocation rendered as a collapsible nested
	// agent card: Tool holds the resolved target (profile id or agent name), Text
	// the delegated task, Output the subagent's final reply, and SubSteps the
	// subagent's own activity trace (its tool calls, thinking, etc.) gathered in an
	// isolated context. Persisted so the nested trace survives reload.
	StepSubagent StepKind = "subagent"
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
	// Tool payload. Tool is the BARE name (namespace stripped) so the UI renders the
	// same clean cards as the native path. CallName is the EXACT name the provider
	// used — namespaced on the claude-cli path (mcp__tionswarm_extended__list_tasks) —
	// set only when it differs from Tool. The recent-tool-activity recap fed back to
	// the model uses CallName so the model sees the real callable name and does not
	// re-call the bare form (which the CLI rejects with "No such tool available").
	Tool     string          `json:"tool,omitempty"`
	CallName string          `json:"callName,omitempty"`
	Input    json.RawMessage `json:"input,omitempty"`
	Output   string          `json:"output,omitempty"`
	IsError  bool            `json:"isError,omitempty"`
	// Options are the suggested clickable answers for a StepAsk prompt (optional;
	// the user may always type a free-text answer instead).
	Options []string `json:"options,omitempty"`
	// Questions carries a MULTI-question StepAsk prompt (each with its own optional
	// options); when set, the UI renders all questions together in one card and the
	// user answers them at once. Text/Options stay the single-question form used by
	// the permission/confirm cards and one-question asks.
	Questions []tools.AskQuestion `json:"questions,omitempty"`
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
	// SubSteps carries the nested activity trace of a StepSubagent step — the
	// subagent's own tool calls / thinking, captured in its isolated context.
	SubSteps []TurnStep `json:"subSteps,omitempty"`
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
	tool := t.Tool
	if s := strings.TrimPrefix(tool, interactionToolPrefix); s != tool {
		tool = s
	} else {
		tool = strings.TrimPrefix(tool, extendedToolPrefix)
	}
	// Preserve the EXACT (namespaced) name the provider used when we stripped a
	// prefix, so the recent-tool-activity recap can show the real callable name and
	// the model doesn't re-call the bare form next turn (CLI rejects it). Native /
	// already-bare tools leave CallName empty (Tool alone is the callable name).
	callName := ""
	if tool != t.Tool {
		callName = t.Tool
	}
	st := TurnStep{
		Kind:     StepKind(t.Kind),
		Text:     t.Text,
		Tool:     tool,
		CallName: callName,
		Input:    t.Input,
		Output:   t.Output,
		IsError:  t.IsError,
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

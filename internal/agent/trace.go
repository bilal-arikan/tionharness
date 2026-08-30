package agent

import (
	"encoding/json"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// interactionToolPrefix / extendedToolPrefix are the two MCP namespaces the
// Interaction MCP server uses (core eager tier vs extended deferred tier). The CLI
// reports its tools namespaced (mcp__tionharness_interaction__ask_user,
// mcp__tionharness_extended__create_agent); we strip either so the persisted trace shows
// the bare tool name and renders with the same cards as the native tool path (todo
// checklist, artifact card, ask).
const (
	interactionToolPrefix = "mcp__tionharness_interaction__"
	extendedToolPrefix    = "mcp__tionharness_extended__"
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
	// StepToolDelta is legacy: only for previously persisted sessions. New live
	// tool output uses StepTool frames with Append set instead.
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
	// StepContextChange announces that the session's frozen static context (prompt
	// epoch snapshot) drifted from live state mid-session: the user edited the
	// agent persona/instructions, changed skills, toggled a capability, or the
	// tool catalog changed. Text is the headline; Areas carries the per-block
	// added/removed diff. Emitted once per drift episode; persisted so the chat
	// history shows when a change landed. The change only takes full effect after
	// a context refresh (the frozen prefix is kept byte-stable until then to
	// preserve the prompt cache).
	StepContextChange StepKind = "context_change"
	// StepCacheBreak announces that this turn lost the session's warm prompt-cache
	// prefix and had to re-pay it cold: Reason carries the attributed cause
	// (model-changed / prompt-or-tools-changed), Text the human explanation and
	// ColdTokens the re-written prefix size. Only the "something changed" causes are
	// carded — a TTL/eviction break is the normal cost of a pause and would be pure
	// noise inline (it stays in the debug journal and the per-message panel).
	StepCacheBreak StepKind = "cache_break"
	// StepCompaction announces that the session's history was folded into the
	// rolling summary to stay inside the context budget: Text is the human-readable
	// headline (kept for clients that predate this kind), FoldedMsgs how many
	// messages were folded, BeforeTokens/AfterTokens the context footprint around
	// the fold and Trigger what caused it (auto / manual / reactive). Persisted so
	// the chat history shows where context was lost.
	StepCompaction StepKind = "compaction"
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
	// used — namespaced on the claude-cli path (mcp__tionharness_extended__list_tasks) —
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
	// ID identifies a live card. A later step with the same ID replaces it unless
	// Append is set; a non-running replacement closes the card.
	ID string `json:"id,omitempty"`
	// Ref is the target live-card ID a StepTombstone retracts when no final card
	// will arrive (for example, cancellation).
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
	// Running marks a partial live card. A later step with the same ID replaces it.
	// Never set on a persisted step.
	Running bool `json:"running,omitempty"`
	// Append adds Text/Output to the existing live card with the same ID instead
	// of replacing it. Append frames are ephemeral and never persisted.
	Append bool `json:"append,omitempty"`
	// Batch groups tool steps born from ONE provider response that carried
	// multiple parallel tool calls: all of them share the same 1-based group id
	// (unique within the turn), so the UI can render them as one "N parallel
	// calls" cluster. 0 (omitted) = a lone call, no grouping. Set by the native
	// tool loop and the claude-cli stream parser alike.
	Batch int `json:"batch,omitempty"`
	// Structured Codex collab metadata. Optional for persisted-step compatibility.
	Operation  string   `json:"operation,omitempty"`
	Target     []string `json:"target,omitempty"`
	Status     string   `json:"status,omitempty"`
	DurationMs int64    `json:"durationMs,omitempty"`
	Summary    string   `json:"summary,omitempty"`
	// Areas carries the per-block added/removed diff for a StepContextChange step
	// (the prompt-epoch drift). Added/Removed above hold the rollup counts.
	Areas []ContextArea `json:"areas,omitempty"`
	// ColdTokens is the prefix size a StepCacheBreak step had to re-pay cold.
	ColdTokens int `json:"coldTokens,omitempty"`
	// StepCompaction payload: how many messages were folded into the rolling
	// summary, the context footprint before/after the fold and what triggered it
	// ("auto" = budget threshold, "manual" = /compact, "reactive" = overflow
	// recovery).
	FoldedMsgs   int    `json:"foldedMsgs,omitempty"`
	BeforeTokens int    `json:"beforeTokens,omitempty"`
	AfterTokens  int    `json:"afterTokens,omitempty"`
	Trigger      string `json:"trigger,omitempty"`
	// Source is "tionharness" or "cli-native"; Provider names the CLI transport;
	// SessionAction records resume/native-compact/restart-summary. Open strings
	// preserve future backend values instead of silently dropping them.
	Source        string `json:"source,omitempty"`
	Provider      string `json:"provider,omitempty"`
	SessionAction string `json:"sessionAction,omitempty"`
	// Optimizer records that an external token-optimizer (sqz / rtk) shrank this
	// shell step's output before it re-entered the model's context, so the UI can
	// show a chip instead of the rewrite being invisible. nil = untouched.
	Optimizer *tools.ShellOptimization `json:"optimizer,omitempty"`
}

// ContextChangeStep builds the persisted/streamed TurnStep for a prompt-epoch
// drift, from the computed diff. Returns a zero step when the change is empty.
func ContextChangeStep(c *ContextChange) TurnStep {
	if c.Empty() {
		return TurnStep{}
	}
	return TurnStep{
		Kind:    StepContextChange,
		Text:    c.Summary(),
		Added:   c.Added,
		Removed: c.Removed,
		Areas:   c.Areas,
	}
}

// CacheBreakStep builds the persisted TurnStep for an attributed prompt-cache
// break. Returns a zero step for a nil break so callers can pass a consume result
// straight through.
func CacheBreakStep(b *CacheBreak) TurnStep {
	if b == nil || b.Reason == "" {
		return TurnStep{}
	}
	return TurnStep{
		Kind:       StepCacheBreak,
		Reason:     b.Reason,
		Text:       b.Detail,
		ColdTokens: b.ColdTokens,
	}
}

// todoStepItems resolves the checklist a todo_write call represents: the
// full-replace form carries it in the input; the compact `set` form carries the
// server-merged list in the tool RESULT (same {"todos":[...]} shape inside the
// JSON confirmation).
func todoStepItems(input json.RawMessage, output string) []TodoItem {
	if todos := parseTodos(input); len(todos) > 0 {
		return todos
	}
	return parseTodos(json.RawMessage(output))
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
		ID:            t.ID,
		Ref:           t.Ref,
		Running:       t.Running,
		Kind:          StepKind(t.Kind),
		Text:          t.Text,
		Tool:          tool,
		CallName:      callName,
		Input:         t.Input,
		Output:        t.Output,
		IsError:       t.IsError,
		Batch:         t.Batch,
		Operation:     t.Operation,
		Target:        append([]string(nil), t.Target...),
		Status:        t.Status,
		DurationMs:    t.DurationMs,
		Summary:       t.Summary,
		Source:        t.Source,
		Provider:      t.Provider,
		SessionAction: t.SessionAction,
		FoldedMsgs:    t.FoldedMsgs,
		BeforeTokens:  t.BeforeTokens,
		AfterTokens:   t.AfterTokens,
		Trigger:       t.Trigger,
	}
	if st.Kind == StepTool && !st.IsError && tool == "todo_write" {
		if todos := todoStepItems(t.Input, t.Output); len(todos) > 0 {
			st.Kind = StepTodo
			st.Todos = todos
		}
	}
	return st
}

// traceStepToTurnStep is the runtime-bound form: the pure mapping plus the
// token-optimizer chip, which only the runtime can resolve (the CLI ran the shell
// through our bridge in a different call stack, so the optimization is recovered
// from the output-keyed log rather than a ctx sink). A miss leaves Optimizer nil.
func (r *Runtime) traceStepToTurnStep(t providers.TraceStep) TurnStep {
	st := traceStepToTurnStep(t)
	if st.Kind == StepTool {
		st.Optimizer = r.optLog.lookup(st.Output)
	}
	return st
}

// traceToSteps converts a provider-produced trace (claude CLI stream-json) into
// the agent-level TurnStep records the chat UI renders.
func (r *Runtime) traceToSteps(tr []providers.TraceStep) []TurnStep {
	if len(tr) == 0 {
		return nil
	}
	steps := make([]TurnStep, 0, len(tr))
	for _, t := range tr {
		steps = append(steps, r.traceStepToTurnStep(t))
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

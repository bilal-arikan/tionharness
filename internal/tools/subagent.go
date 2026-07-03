package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// RunAgentResult is the outcome of a run_subagent invocation. For a synchronous
// run Reply holds the subagent's final text; for an async (detached) run Async is
// true and SessionID points at the background session.
type RunAgentResult struct {
	AgentName string
	Reply     string
	SessionID string
	Async     bool
}

// RunAgentSpec is the parsed run_subagent request. Unset axes take their default
// (sync, isolated) so the common case — a clean, blocking subagent — needs only
// target + task.
type RunAgentSpec struct {
	Target  string // profile id (ephemeral) OR existing agent name/id
	Task    string
	Wait    string // "sync" (default) | "async"
	Context string // "isolated" (default) | "inherited"
	Model   string

	// Structured task contract (all optional). When any is set, the runner injects
	// a "Task contract" block into the subagent's system prompt so the work has a
	// clear objective, a required output shape and explicit scope limits — the four
	// elements Anthropic's multi-agent guidance calls for to avoid duplicated work
	// and gaps. Empty fields are omitted, so the plain-task path stays unchanged.
	Objective    string // the specific goal the subagent must accomplish
	OutputFormat string // how the reply must be structured (the caller sees only this)
	Boundaries   string // explicit scope limits — what to exclude / not touch
}

// RunAgentFunc executes one (sub)agent run. It is implemented in the agent
// package (which owns the runtime, the agent store and the call-graph guards) and
// injected via context so built-in tools — which must not import the agent
// package — can launch a subagent. It enforces all loop-protection (depth limit,
// cycle/visited-set, per-turn budget, concurrency) and returns a plain error when
// a guard refuses, so the model can adapt instead of the turn aborting.
type RunAgentFunc func(ctx context.Context, spec RunAgentSpec) (RunAgentResult, error)

// runAgentKey keys the RunAgentFunc on a request context.
type runAgentKey struct{}

// WithRunAgent attaches a run-agent runner to ctx so the run_subagent tool can
// launch a subagent for this turn. Mirrors WithDiffSink/WithAsker: kept in the
// tools package so built-ins reach it without importing the agent package.
func WithRunAgent(ctx context.Context, fn RunAgentFunc) context.Context {
	return context.WithValue(ctx, runAgentKey{}, fn)
}

// RunAgentFrom returns the runner attached to ctx, or nil when subagents are not
// wired (e.g. provider-driven CLI loops, or the feature disabled).
func RunAgentFrom(ctx context.Context) RunAgentFunc {
	fn, _ := ctx.Value(runAgentKey{}).(RunAgentFunc)
	return fn
}

// runSubagentInput is the argument shape for the run_subagent tool.
type runSubagentInput struct {
	Target       string `json:"target"`
	Task         string `json:"task"`
	Wait         string `json:"wait"`
	Context      string `json:"context"`
	Model        string `json:"model"`
	Objective    string `json:"objective"`
	OutputFormat string `json:"output_format"`
	Boundaries   string `json:"boundaries"`
}

// RunSubagentTool launches an isolated subagent to carry out a self-contained
// task and (by default) returns only its final result — so a large sub-task's
// tool output never floods the caller's own context. The single generic
// primitive for all agent-to-agent work: pick a built-in profile (a throwaway
// typed worker) or name an existing workspace agent; run it now and wait (sync)
// or detached in the background (async); give it a clean context (isolated) or
// the current conversation (inherited). Emit several run_subagent calls in one
// turn to fan work out in parallel.
type RunSubagentTool struct{}

// NewRunSubagentTool constructs the run_subagent tool.
func NewRunSubagentTool() RunSubagentTool { return RunSubagentTool{} }

func (RunSubagentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "run_subagent",
		Description: "Launch an isolated subagent for a self-contained task and get back ONLY its final " +
			"result — its intermediate tool output never enters your context. `target` is a built-in " +
			"profile (\"explore\" = read-only search, \"coder\" = write/edit code, \"reviewer\" = read-only " +
			"review) or the name/id of an existing workspace agent. Defaults: runs now and waits (sync) " +
			"with a clean context (isolated). Call several times in one turn to fan work out in parallel. " +
			"For LONG tasks (writing many files, running a test suite, multi-minute work) prefer " +
			"`wait:\"async\"` with an EXISTING agent target, then poll the filesystem (the source of truth) " +
			"for the expected output — a long sync call can hit the CLI's tool-call timeout (\"The operation " +
			"timed out.\") even though the work keeps running in the background. Use sync only for quick, " +
			"bounded work. " +
			"Give an `objective`, `output_format` and `boundaries` for best results — vague tasks cause gaps.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "target": { "type": "string", "description": "Profile id (\"explore\" | \"coder\" | \"reviewer\") or an existing agent's name/id." },
    "task": { "type": "string", "description": "A clear, self-contained instruction. The subagent sees nothing of your context unless context=inherited." },
    "wait": { "type": "string", "enum": ["sync", "async"], "description": "\"sync\" (default): run now, return the reply — best for quick, bounded work. \"async\": detach into a background session (EXISTING agents only, not profiles); prefer this for long/multi-minute tasks and poll the filesystem for output, since a long sync call can hit the CLI tool-call timeout." },
    "context": { "type": "string", "enum": ["isolated", "inherited"], "description": "\"isolated\" (default): clean context. \"inherited\": also pass the current conversation." },
    "model": { "type": "string", "description": "Optional model id override." },
    "objective": { "type": "string", "description": "Optional. One-sentence goal — prevents scope drift." },
    "output_format": { "type": "string", "description": "Optional. How the reply must be structured (e.g. \"bulleted file:line list\", \"JSON with keys x,y\")." },
    "boundaries": { "type": "string", "description": "Optional. Scope limits — what to exclude / NOT touch." }
  },
  "required": ["target", "task"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"target":"explore","task":"Map how sessions are persisted.","objective":"Locate every read/write of session JSONL files","output_format":"bulleted file:line list, one per call site","boundaries":"only internal/db; do not read frontend; no code edits"}`),
		},
	}
}

func (RunSubagentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	in, err := parseInput[runSubagentInput]("run_subagent", input)
	if err != nil {
		return "", err
	}
	spec := RunAgentSpec{
		Target:       strings.TrimSpace(in.Target),
		Task:         strings.TrimSpace(in.Task),
		Wait:         strings.ToLower(strings.TrimSpace(in.Wait)),
		Context:      strings.ToLower(strings.TrimSpace(in.Context)),
		Model:        strings.TrimSpace(in.Model),
		Objective:    strings.TrimSpace(in.Objective),
		OutputFormat: strings.TrimSpace(in.OutputFormat),
		Boundaries:   strings.TrimSpace(in.Boundaries),
	}
	if spec.Target == "" || spec.Task == "" {
		return "", fmt.Errorf("both \"target\" and \"task\" are required")
	}
	run := RunAgentFrom(ctx)
	if run == nil {
		return "", fmt.Errorf("subagents are not available in this context")
	}
	res, err := run(ctx, spec)
	if err != nil {
		return "", err
	}
	if res.Async {
		return fmt.Sprintf("Started subagent %q in the background (session %s). It runs on its own; you do not wait for it.", res.AgentName, res.SessionID), nil
	}
	return fmt.Sprintf("Result from subagent %q:\n\n%s", res.AgentName, res.Reply), nil
}

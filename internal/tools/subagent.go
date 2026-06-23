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
	Target  string `json:"target"`
	Task    string `json:"task"`
	Wait    string `json:"wait"`
	Context string `json:"context"`
	Model   string `json:"model"`
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
		Description: "Launch an isolated subagent to do a self-contained task and get back ONLY its final " +
			"result — its intermediate tool output never enters your context, keeping your turn lean. " +
			"`target` is either a built-in profile (\"explore\" = read-only search/discovery, \"coder\" = " +
			"write/edit code, \"reviewer\" = read-only independent review) for a throwaway worker, OR the " +
			"name/id of an existing workspace agent. Defaults: runs now and waits (`wait`:\"sync\") with a " +
			"clean context (`context`:\"isolated\"). Set `wait`:\"async\" to detach it into a background " +
			"session (existing agents only). Set `context`:\"inherited\" to let it see the current " +
			"conversation. Call this several times in one turn to run subagents in parallel. Keep nesting " +
			"shallow; prefer doing trivial work yourself.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "target": { "type": "string", "description": "Profile id (\"explore\" | \"coder\" | \"reviewer\") for an ephemeral worker, or the name/id of an existing agent." },
    "task": { "type": "string", "description": "A clear, self-contained instruction. The subagent does not see your context unless context=inherited." },
    "wait": { "type": "string", "enum": ["sync", "async"], "description": "\"sync\" (default): run now and return the reply. \"async\": detach into a background session (existing agents only)." },
    "context": { "type": "string", "enum": ["isolated", "inherited"], "description": "\"isolated\" (default): clean context, only the task. \"inherited\": also pass the current conversation." },
    "model": { "type": "string", "description": "Optional model id to use instead of the target's default." }
  },
  "required": ["target", "task"],
  "additionalProperties": false
}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"target":"explore","task":"Find every place the auth token is validated and list file:line for each."}`),
			json.RawMessage(`{"target":"reviewer","task":"Review internal/agent/subagent.go for race conditions; report only real issues.","context":"isolated"}`),
		},
	}
}

func (RunSubagentTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in runSubagentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("run_subagent", err)
	}
	spec := RunAgentSpec{
		Target:  strings.TrimSpace(in.Target),
		Task:    strings.TrimSpace(in.Task),
		Wait:    strings.ToLower(strings.TrimSpace(in.Wait)),
		Context: strings.ToLower(strings.TrimSpace(in.Context)),
		Model:   strings.TrimSpace(in.Model),
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

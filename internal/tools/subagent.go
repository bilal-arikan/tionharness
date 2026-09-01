package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// RunAgentResult is the outcome of a run_subagent invocation: Reply holds the
// subagent's final text, gathered once its run has finished.
type RunAgentResult struct {
	AgentName string
	Reply     string
	// Artifacts are the artifacts the subagent produced during this run, owned by
	// its own child session. They are handed back as REFERENCES, not content: the
	// point of delegation is that the sub-task's output never floods the caller's
	// context, and a large document would do exactly that. The caller reads one by
	// id with read_artifact when it actually needs the body.
	Artifacts []SubagentArtifact

	// FanOut carries one outcome per task when the call was a fan-out, in input
	// order. Non-empty FanOut means AgentName/Reply/Artifacts above are unused —
	// there is no single reply to put there.
	FanOut   []FanOutOutcome
	Strategy string
}

// SubagentArtifact is one artifact reference returned to the delegating caller.
type SubagentArtifact struct {
	ID    string
	Title string
	Kind  string
}

// RunAgentSpec is the parsed run_subagent request. An unset axis takes its
// default (isolated) so the common case — a clean, blocking subagent — needs only
// target + task.
type RunAgentSpec struct {
	Target  string // profile id (ephemeral) OR existing agent name/id
	Task    string
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

	// RetryOf names a finished child session this run replaces. The new run is a
	// fresh child session linked back to that attempt, so the failed transcript
	// survives for inspection instead of being overwritten. The caller may pair it
	// with a different target/model/context — retrying the same task a different
	// way is the point.
	RetryOf string

	// Fan-out. When Tasks is non-empty this call runs SEVERAL subagents and the
	// single-task fields above act as the per-leg defaults. Strategy decides how
	// the legs are aggregated; MaxConcurrency caps how many run at once (0 =
	// DefaultFanOutConcurrency). Tasks and Task are mutually exclusive.
	Tasks          []RunAgentTask
	Strategy       string
	MaxConcurrency int
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
	Context      string `json:"context"`
	Model        string `json:"model"`
	Objective    string `json:"objective"`
	OutputFormat string `json:"output_format"`
	Boundaries   string `json:"boundaries"`
	RetryOf      string `json:"retry_of"`

	// Wait is a dead axis kept only for compatibility. The schema is
	// additionalProperties:false, so a frozen prompt epoch that still emits
	// wait:"sync" would hard-fail the whole call if the field were dropped. It is
	// accepted and ignored; the removed "async" value is refused loudly rather
	// than silently downgraded to a blocking run.
	Wait string `json:"wait"`

	// Fan-out axes. Tasks is the multi-task form; the single-task fields above
	// become its per-leg defaults.
	Tasks          []fanOutTaskInput `json:"tasks"`
	Strategy       string            `json:"strategy"`
	MaxConcurrency int               `json:"max_concurrency"`
}

// RunSubagentTool launches an isolated subagent to carry out a self-contained
// task and returns only its final result — so a large sub-task's tool output
// never floods the caller's own context. The single generic primitive for all
// agent-to-agent work: pick a built-in profile (a throwaway typed worker) or name
// an existing workspace agent; give it a clean context (isolated) or the current
// conversation (inherited). The call blocks until the subagent is done. Emit
// several run_subagent calls in one turn to fan work out in parallel.
type RunSubagentTool struct{}

// NewRunSubagentTool constructs the run_subagent tool.
func NewRunSubagentTool() RunSubagentTool { return RunSubagentTool{} }

func (RunSubagentTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "run_subagent",
		// The FIRST LINE is the delegation nudge: run_subagent is SUMMARY-tier (lazy),
		// so the load-on-demand catalog shows exactly this line (lazyDescription takes
		// the first non-empty line, capped at 200 chars). Keep it a complete sentence
		// under that cap — it is the only thing reminding the model delegation exists.
		// Everything after it ships only once the schema is activated.
		Description: "Delegate a self-contained task to an isolated subagent and get back ONLY its final " +
			"result — its intermediate tool output never enters your context.\n" +
			"`target` is a built-in " +
			"profile (\"explore\" read-only search, \"planner\" read-only planning, \"coder\" edits code, " +
			"\"reviewer\" read-only review) " +
			"or an existing agent's name/id. Default context: isolated. The call BLOCKS until the subagent " +
			"finishes. Call several times in one turn to fan work out in parallel. A long run can hit the " +
			"tool-call timeout, so split long work into several smaller self-contained subagent calls; for " +
			"genuinely long-running work, let a coordinator drive it (`set_coordinator_mode` then " +
			"`spawn_worker`). Set `objective`/`output_format`/`boundaries` for best results — vague " +
			"tasks cause gaps.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "target": { "type": "string", "description": "Profile id (\"explore\" | \"planner\" | \"coder\" | \"reviewer\") or an existing agent's name/id." },
    "task": { "type": "string", "description": "A clear, self-contained instruction. The subagent sees nothing of your context unless context=inherited." },
    "context": { "type": "string", "enum": ["isolated", "inherited"], "description": "\"isolated\" (default): clean context. \"inherited\": also pass the current conversation." },
    "model": { "type": "string", "description": "Optional model id override." },
    "objective": { "type": "string", "description": "Optional one-sentence goal — prevents scope drift." },
    "output_format": { "type": "string", "description": "Optional reply structure (e.g. \"bulleted file:line list\")." },
    "boundaries": { "type": "string", "description": "Optional scope limits — what to exclude / NOT touch." },
    "retry_of": { "type": "string", "description": "Session id of a FINISHED subagent run of yours that this call retries. The failed transcript is kept and linked; pair it with a different target/model/context to retry the task a different way." },
    "wait": { "type": "string", "enum": ["sync"], "description": "Deprecated and ignored — run_subagent is always synchronous." },
    "tasks": {
      "type": "array",
      "description": "Fan-out: run SEVERAL subagents from one call. Mutually exclusive with \"task\". The top-level target/context/model/objective/output_format/boundaries become the per-task defaults, so the common shape — one target, several tasks — needs no repetition.",
      "items": {
        "type": "object",
        "properties": {
          "target": { "type": "string", "description": "Overrides the top-level target for this task." },
          "task": { "type": "string", "description": "This task's self-contained instruction." },
          "context": { "type": "string", "enum": ["isolated", "inherited"] },
          "model": { "type": "string" },
          "objective": { "type": "string" },
          "output_format": { "type": "string" },
          "boundaries": { "type": "string" }
        },
        "required": ["task"],
        "additionalProperties": false
      }
    },
    "strategy": { "type": "string", "enum": ["all", "first-success"], "description": "How a \"tasks\" fan-out is aggregated. \"all\" (default): wait for every task, report each in input order (a failed task is reported, it does not fail the call). \"first-success\": return as soon as one task succeeds and cancel the rest — use it when the tasks are alternative routes to the SAME answer." },
    "max_concurrency": { "type": "integer", "minimum": 1, "description": "How many fan-out tasks run at once (default 4). The per-turn delegation budget still applies on top." }
  },
  "required": ["target"],
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
		Context:      strings.ToLower(strings.TrimSpace(in.Context)),
		Model:        strings.TrimSpace(in.Model),
		Objective:    strings.TrimSpace(in.Objective),
		OutputFormat: strings.TrimSpace(in.OutputFormat),
		Boundaries:   strings.TrimSpace(in.Boundaries),
		RetryOf:      strings.TrimSpace(in.RetryOf),
	}
	if spec.Target == "" && len(in.Tasks) == 0 {
		return "", fmt.Errorf("\"target\" is required")
	}
	if spec.Task == "" && len(in.Tasks) == 0 {
		return "", fmt.Errorf("either \"task\" (one subagent) or \"tasks\" (a fan-out) is required")
	}
	// An out-of-enum axis is refused, never defaulted: context="inherit" would run
	// ISOLATED silently — the caller would get the exact opposite of what it asked
	// for with no way to notice.
	if !oneOfEnum(spec.Context, "isolated", "inherited") {
		return "", fmt.Errorf("\"context\" must be one of isolated, inherited; got %q", spec.Context)
	}
	// "wait" no longer exists as a behaviour; only the historical "sync" value is
	// tolerated so a frozen prompt keeps working. "async" is refused instead of
	// ignored — a caller that asked for a detached run must not silently get a
	// blocking one.
	if w := strings.ToLower(strings.TrimSpace(in.Wait)); w != "" && w != "sync" {
		return "", fmt.Errorf(`"wait":"async" is no longer supported — run_subagent is always synchronous; break the work into smaller sync calls or hand long-running work to a coordinator worker`)
	}
	spec, err = buildFanOutSpec(in, spec)
	if err != nil {
		return "", err
	}
	run := RunAgentFrom(ctx)
	if run == nil {
		return "", fmt.Errorf("subagents are not available in this context")
	}
	res, err := run(ctx, spec)
	if err != nil {
		return "", err
	}
	return FormatRunAgentResult(res)
}

// FormatRunAgentResult renders a finished run for the calling model. Split out of
// Call so the wording — which is the entire interface the caller sees — can be
// asserted without standing up a runtime.
func FormatRunAgentResult(res RunAgentResult) (string, error) {
	if len(res.FanOut) > 0 {
		return formatFanOut(res), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Result from subagent %q:\n\n%s", res.AgentName, res.Reply)
	if len(res.Artifacts) > 0 {
		b.WriteString("\n\nArtifacts it produced (owned by its run; read one with read_artifact):")
		for _, a := range res.Artifacts {
			fmt.Fprintf(&b, "\n- %s — %s (%s)", a.ID, a.Title, a.Kind)
		}
	}
	return b.String(), nil
}

// oneOfEnum reports whether v is empty (unset — the caller takes the default) or one
// of the allowed values. Already trimmed and lower-cased by the caller.
func oneOfEnum(v string, allowed ...string) bool {
	if v == "" {
		return true
	}
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

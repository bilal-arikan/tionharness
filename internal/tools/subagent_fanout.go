package tools

import (
	"fmt"
	"strings"
)

// Fan-out strategies. The set is deliberately small: these two are the ones whose
// meaning is unambiguous for free-text replies. Rank-and-pick strategies
// (majority, reviewer-selects) need a judging turn to define "same answer" and are
// tracked separately.
const (
	// StrategyAll waits for every task and reports them all, in input order.
	StrategyAll = "all"
	// StrategyFirstSuccess returns as soon as one task succeeds and cancels the
	// rest.
	StrategyFirstSuccess = "first-success"
)

// DefaultFanOutConcurrency caps how many tasks of one fan-out run at once when the
// caller does not say. It is a throughput knob, not a safety one — the per-turn
// delegation budget (DelegationMaxCalls) is what actually bounds total spend, and
// it applies whether the tasks run four at a time or all at once.
const DefaultFanOutConcurrency = 4

// RunAgentTask is one leg of a fan-out. Every axis is optional except Task: an
// unset field falls back to the call's top-level value, so the common shape —
// same target, several different tasks — needs no repetition.
type RunAgentTask struct {
	Target       string
	Task         string
	Context      string
	Model        string
	Objective    string
	OutputFormat string
	Boundaries   string
}

// FanOutOutcome is one leg's result. Exactly one of Reply/Error is meaningful;
// Skipped marks a leg that never ran (cancelled once first-success had a winner),
// which is NOT a failure and must not be reported as one.
type FanOutOutcome struct {
	Index     int
	Target    string
	AgentName string
	Reply     string
	Error     string
	Skipped   bool
	Artifacts []SubagentArtifact
}

// fanOutTaskInput is the wire shape of one element of the "tasks" array.
type fanOutTaskInput struct {
	Target       string `json:"target"`
	Task         string `json:"task"`
	Context      string `json:"context"`
	Model        string `json:"model"`
	Objective    string `json:"objective"`
	OutputFormat string `json:"output_format"`
	Boundaries   string `json:"boundaries"`
}

// buildFanOutSpec validates the multi-task form of a run_subagent call and folds
// the top-level defaults into each leg.
//
// Every conflict is an error rather than a precedence rule. A call that carries
// both "task" and "tasks" has two readings and picking one silently would run
// work the caller did not ask for; the same goes for fan-out-only axes attached to
// a single-task call, which would otherwise look accepted and do nothing.
func buildFanOutSpec(in runSubagentInput, base RunAgentSpec) (RunAgentSpec, error) {
	strategy := strings.ToLower(strings.TrimSpace(in.Strategy))
	if len(in.Tasks) == 0 {
		if strategy != "" {
			return RunAgentSpec{}, fmt.Errorf("\"strategy\" only applies to a \"tasks\" fan-out; a single \"task\" has nothing to aggregate")
		}
		if in.MaxConcurrency != 0 {
			return RunAgentSpec{}, fmt.Errorf("\"max_concurrency\" only applies to a \"tasks\" fan-out")
		}
		return base, nil
	}
	if base.Task != "" {
		return RunAgentSpec{}, fmt.Errorf("pass either \"task\" (one subagent) or \"tasks\" (a fan-out), not both")
	}
	if base.RetryOf != "" {
		return RunAgentSpec{}, fmt.Errorf("\"retry_of\" retries ONE finished run; retry each leg with its own call instead of a \"tasks\" fan-out")
	}
	if strategy == "" {
		strategy = StrategyAll
	}
	if strategy != StrategyAll && strategy != StrategyFirstSuccess {
		return RunAgentSpec{}, fmt.Errorf("\"strategy\" must be one of %s, %s; got %q", StrategyAll, StrategyFirstSuccess, strategy)
	}
	if in.MaxConcurrency < 0 {
		return RunAgentSpec{}, fmt.Errorf("\"max_concurrency\" must be positive; got %d", in.MaxConcurrency)
	}

	legs := make([]RunAgentTask, 0, len(in.Tasks))
	for i, t := range in.Tasks {
		leg := RunAgentTask{
			Target:       orFallback(t.Target, base.Target),
			Task:         strings.TrimSpace(t.Task),
			Context:      strings.ToLower(orFallback(t.Context, base.Context)),
			Model:        orFallback(t.Model, base.Model),
			Objective:    orFallback(t.Objective, base.Objective),
			OutputFormat: orFallback(t.OutputFormat, base.OutputFormat),
			Boundaries:   orFallback(t.Boundaries, base.Boundaries),
		}
		if leg.Task == "" {
			return RunAgentSpec{}, fmt.Errorf("tasks[%d]: \"task\" is required", i)
		}
		if leg.Target == "" {
			return RunAgentSpec{}, fmt.Errorf("tasks[%d]: no \"target\" — set one on the task or a shared one at the top level", i)
		}
		if !oneOfEnum(leg.Context, "isolated", "inherited") {
			return RunAgentSpec{}, fmt.Errorf("tasks[%d]: \"context\" must be one of isolated, inherited; got %q", i, leg.Context)
		}
		legs = append(legs, leg)
	}
	base.Tasks = legs
	base.Strategy = strategy
	base.MaxConcurrency = in.MaxConcurrency
	return base, nil
}

// orFallback returns the trimmed value, or the trimmed fallback when unset.
func orFallback(v, fallback string) string {
	if s := strings.TrimSpace(v); s != "" {
		return s
	}
	return strings.TrimSpace(fallback)
}

// formatFanOut renders a completed fan-out for the calling model.
//
// Legs are printed in INPUT order regardless of the order they finished in:
// the caller numbered them, and a result list that reshuffles itself run to run
// cannot be referred to ("the second one") or diffed between turns.
func formatFanOut(res RunAgentResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fan-out of %d subagent tasks (strategy: %s)", len(res.FanOut), res.Strategy)
	for _, o := range res.FanOut {
		switch {
		case o.Skipped:
			fmt.Fprintf(&b, "\n\n--- [%d] %s — SKIPPED (another task already succeeded)", o.Index+1, o.Target)
		case o.Error != "":
			fmt.Fprintf(&b, "\n\n--- [%d] %s — FAILED: %s", o.Index+1, o.Target, o.Error)
		default:
			fmt.Fprintf(&b, "\n\n--- [%d] %s:\n%s", o.Index+1, o.AgentName, o.Reply)
			for _, a := range o.Artifacts {
				fmt.Fprintf(&b, "\n  artifact %s — %s (%s)", a.ID, a.Title, a.Kind)
			}
		}
	}
	return b.String()
}

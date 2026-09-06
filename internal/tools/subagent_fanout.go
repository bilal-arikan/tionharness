package tools

import (
	"fmt"
	"strings"
)

// Collecting fan-out strategies: they report the legs, they do not judge them.
// The rank-and-pick strategies that DO elect a winner (majority,
// reviewer-selects) live in subagent_aggregate.go, together with the definitions
// of "the same answer" and "no winner" they each need.
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

	// Winner marks the leg a rank-and-pick strategy elected as THE answer. Always
	// false for the collecting strategies, which elect nobody.
	Winner bool
	// Agreement is how many successful legs gave this leg's answer, counting
	// itself. Only StrategyMajority fills it in; 0 everywhere else.
	Agreement int
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
	if err := validateStrategy(strategy); err != nil {
		return RunAgentSpec{}, err
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
	if err := validateAggregateStrategy(strategy, legs); err != nil {
		return RunAgentSpec{}, err
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
//
// A rank-and-pick strategy prints only the WINNER's reply in full. Its whole
// purpose is to turn N answers into one, and pasting the losing replies back
// would hand the caller the same pile of text it delegated in order to avoid.
// The losers are still listed — one status line plus any artifact they produced —
// so nothing that ran or was written disappears from the report.
func formatFanOut(res RunAgentResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fan-out of %d subagent tasks (strategy: %s)", len(res.FanOut), res.Strategy)
	if verdict := fanOutVerdict(res); verdict != "" {
		b.WriteString(verdict)
	}
	condensed := isRankAndPick(res.Strategy)
	for _, o := range res.FanOut {
		switch {
		case o.Skipped:
			fmt.Fprintf(&b, "\n\n--- [%d] %s — SKIPPED (another task already succeeded)", o.Index+1, o.Target)
		case o.Error != "":
			fmt.Fprintf(&b, "\n\n--- [%d] %s — FAILED: %s", o.Index+1, o.Target, o.Error)
		case condensed && !o.Winner:
			fmt.Fprintf(&b, "\n\n--- [%d] %s — %s", o.Index+1, o.AgentName, notSelectedNote(res.Strategy, o))
			for _, a := range o.Artifacts {
				fmt.Fprintf(&b, "\n  artifact %s — %s (%s)", a.ID, a.Title, a.Kind)
			}
		default:
			fmt.Fprintf(&b, "\n\n--- [%d] %s%s:\n%s", o.Index+1, o.AgentName, winnerNote(res.Strategy, o), o.Reply)
			for _, a := range o.Artifacts {
				fmt.Fprintf(&b, "\n  artifact %s — %s (%s)", a.ID, a.Title, a.Kind)
			}
		}
	}
	return b.String()
}

// fanOutVerdict adds the one-line summary a rank-and-pick strategy owes the
// caller: which leg won and on what grounds.
func fanOutVerdict(res RunAgentResult) string {
	if !isRankAndPick(res.Strategy) {
		return ""
	}
	for _, o := range res.FanOut {
		if !o.Winner {
			continue
		}
		if res.Strategy == StrategyMajority {
			return fmt.Sprintf(" — winner: [%d], agreed on by %d of %d tasks", o.Index+1, o.Agreement, countAnswered(res.FanOut))
		}
		return fmt.Sprintf(" — winner: [%d], chosen by the reviewer", o.Index+1)
	}
	return ""
}

// winnerNote labels the leg whose reply is printed in full.
func winnerNote(strategy string, o FanOutOutcome) string {
	if !o.Winner {
		return ""
	}
	if strategy == StrategyMajority {
		return " — MAJORITY ANSWER"
	}
	return " — SELECTED by the reviewer"
}

// notSelectedNote says why a successful leg's reply is not printed, and — for
// majority — whether it agreed with the winner or dissented, which is the part
// the caller actually needs in order to trust the verdict.
func notSelectedNote(strategy string, o FanOutOutcome) string {
	const suffix = "reply not shown; re-run with strategy \"all\" to read it"
	if strategy == StrategyMajority {
		if o.Agreement > 1 {
			return fmt.Sprintf("agreed with the majority (%s)", suffix)
		}
		return fmt.Sprintf("DISSENTED (%s)", suffix)
	}
	return fmt.Sprintf("not selected (%s)", suffix)
}

// countAnswered is how many legs actually produced an answer — the denominator a
// majority is a majority OF.
func countAnswered(outcomes []FanOutOutcome) int {
	n := 0
	for _, o := range outcomes {
		if o.Error == "" && !o.Skipped {
			n++
		}
	}
	return n
}

package tools

import (
	"strings"
	"testing"
)

// callFanOut runs the tool's argument parsing + fan-out validation the way Call
// does, returning the spec that would reach the runner.
func callFanOut(t *testing.T, in runSubagentInput) (RunAgentSpec, error) {
	t.Helper()
	base := RunAgentSpec{
		Target:       strings.TrimSpace(in.Target),
		Task:         strings.TrimSpace(in.Task),
		Context:      strings.ToLower(strings.TrimSpace(in.Context)),
		Model:        strings.TrimSpace(in.Model),
		Objective:    strings.TrimSpace(in.Objective),
		OutputFormat: strings.TrimSpace(in.OutputFormat),
		Boundaries:   strings.TrimSpace(in.Boundaries),
		RetryOf:      strings.TrimSpace(in.RetryOf),
	}
	return buildFanOutSpec(in, base)
}

// TestFanOutInheritsTopLevelDefaults: the common shape is one target and several
// tasks, so a leg that names nothing but its task must inherit the rest.
func TestFanOutInheritsTopLevelDefaults(t *testing.T) {
	spec, err := callFanOut(t, runSubagentInput{
		Target:       "explore",
		Context:      "isolated",
		Model:        "claude-opus-4-8",
		Boundaries:   "no edits",
		OutputFormat: "bullets",
		Tasks: []fanOutTaskInput{
			{Task: "map sessions"},
			{Task: "map artifacts", Target: "reviewer", Boundaries: "read only"},
		},
	})
	if err != nil {
		t.Fatalf("build fan-out: %v", err)
	}
	if len(spec.Tasks) != 2 {
		t.Fatalf("expected 2 legs, got %d", len(spec.Tasks))
	}
	if spec.Strategy != StrategyAll {
		t.Fatalf("strategy should default to %q, got %q", StrategyAll, spec.Strategy)
	}
	first := spec.Tasks[0]
	if first.Target != "explore" || first.Model != "claude-opus-4-8" ||
		first.Boundaries != "no edits" || first.OutputFormat != "bullets" {
		t.Fatalf("leg 0 should inherit every unset axis, got %+v", first)
	}
	second := spec.Tasks[1]
	if second.Target != "reviewer" || second.Boundaries != "read only" {
		t.Fatalf("leg 1 should override what it names, got %+v", second)
	}
	if second.Model != "claude-opus-4-8" {
		t.Fatalf("leg 1 should still inherit the axes it did NOT name, got %+v", second)
	}
}

// TestFanOutRefusesAmbiguousAndInertCalls: every conflict is an error rather than
// a precedence rule — silently picking one reading would run work the caller did
// not ask for, and an inert axis would look accepted while doing nothing.
func TestFanOutRefusesAmbiguousAndInertCalls(t *testing.T) {
	cases := []struct {
		name string
		in   runSubagentInput
		want string
	}{
		{
			name: "task and tasks together",
			in:   runSubagentInput{Target: "explore", Task: "one", Tasks: []fanOutTaskInput{{Task: "two"}}},
			want: "not both",
		},
		{
			name: "strategy without a fan-out",
			in:   runSubagentInput{Target: "explore", Task: "one", Strategy: "all"},
			want: "only applies to a \"tasks\" fan-out",
		},
		{
			name: "max_concurrency without a fan-out",
			in:   runSubagentInput{Target: "explore", Task: "one", MaxConcurrency: 2},
			want: "only applies to a \"tasks\" fan-out",
		},
		{
			name: "retry_of with a fan-out",
			in:   runSubagentInput{Target: "explore", RetryOf: "SES9", Tasks: []fanOutTaskInput{{Task: "x"}}},
			want: "retries ONE finished run",
		},
		{
			name: "unknown strategy",
			in:   runSubagentInput{Target: "explore", Strategy: "majority", Tasks: []fanOutTaskInput{{Task: "x"}}},
			want: "must be one of all, first-success",
		},
		{
			name: "blank task in a leg",
			in:   runSubagentInput{Target: "explore", Tasks: []fanOutTaskInput{{Task: "x"}, {Task: "  "}}},
			want: "tasks[1]",
		},
		{
			name: "no target anywhere",
			in:   runSubagentInput{Tasks: []fanOutTaskInput{{Task: "x"}}},
			want: "no \"target\"",
		},
		{
			name: "bad context on a leg",
			in:   runSubagentInput{Target: "explore", Tasks: []fanOutTaskInput{{Task: "x", Context: "inherit"}}},
			want: "must be one of isolated, inherited",
		},
		{
			name: "negative concurrency",
			in:   runSubagentInput{Target: "explore", MaxConcurrency: -1, Tasks: []fanOutTaskInput{{Task: "x"}}},
			want: "must be positive",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := callFanOut(t, tc.in)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected an error containing %q, got %v", tc.want, err)
			}
		})
	}
}

// TestSingleTaskCallIsUntouched: the fan-out validator must be a no-op on the
// plain form, or every existing caller changes behaviour.
func TestSingleTaskCallIsUntouched(t *testing.T) {
	spec, err := callFanOut(t, runSubagentInput{Target: "explore", Task: "one"})
	if err != nil {
		t.Fatalf("plain call: %v", err)
	}
	if len(spec.Tasks) != 0 || spec.Strategy != "" || spec.MaxConcurrency != 0 {
		t.Fatalf("a single-task call must gain no fan-out state, got %+v", spec)
	}
}

// TestFormatFanOutKeepsInputOrderAndMarksSkips: the caller numbered the legs, so a
// result list that reshuffles itself cannot be referred to or diffed; and a
// skipped leg must not read as a failed one.
func TestFormatFanOutKeepsInputOrderAndMarksSkips(t *testing.T) {
	out := formatFanOut(RunAgentResult{
		Strategy: StrategyFirstSuccess,
		FanOut: []FanOutOutcome{
			{Index: 0, Target: "explore", Error: "provider unavailable"},
			{Index: 1, Target: "reviewer", AgentName: "Reviewer", Reply: "found it",
				Artifacts: []SubagentArtifact{{ID: "ART3", Title: "Notes", Kind: "markdown"}}},
			{Index: 2, Target: "coder", Skipped: true},
		},
	})
	if !strings.Contains(out, "strategy: first-success") {
		t.Fatalf("header should name the strategy: %s", out)
	}
	iFail := strings.Index(out, "[1] explore")
	iOK := strings.Index(out, "[2] Reviewer")
	iSkip := strings.Index(out, "[3] coder")
	if iFail < 0 || iOK < 0 || iSkip < 0 || !(iFail < iOK && iOK < iSkip) {
		t.Fatalf("legs must appear in input order: %s", out)
	}
	if !strings.Contains(out, "FAILED: provider unavailable") {
		t.Fatalf("a failed leg must carry its error: %s", out)
	}
	if !strings.Contains(out, "SKIPPED") || strings.Contains(out, "[3] coder — FAILED") {
		t.Fatalf("a skipped leg must not read as a failure: %s", out)
	}
	if !strings.Contains(out, "ART3") {
		t.Fatalf("a leg's artifacts must be listed: %s", out)
	}
}

package tools

import (
	"strings"
	"testing"
)

// answered builds a successful leg outcome.
func answered(i int, agent, reply string) FanOutOutcome {
	return FanOutOutcome{Index: i, Target: "explore", AgentName: agent, Reply: reply}
}

// TestMajorityIgnoresFormattingNoiseOnly: the equality key folds case and
// collapses whitespace — the noise that makes two identical answers look
// different — but it does not paraphrase, so differently worded answers stay
// different.
func TestMajorityIgnoresFormattingNoiseOnly(t *testing.T) {
	winner, agreement, err := MajorityWinner([]FanOutOutcome{
		answered(0, "A", "  Yes \n"),
		answered(1, "B", "yes"),
		answered(2, "C", "Yes, definitely"),
	})
	if err != nil {
		t.Fatalf("two legs agreed, expected a majority: %v", err)
	}
	if winner != 0 {
		t.Fatalf("expected leg 0 to win, got %d", winner)
	}
	if agreement[0] != 2 || agreement[1] != 2 {
		t.Fatalf("the agreeing legs must both count 2, got %v", agreement)
	}
	if agreement[2] != 1 {
		t.Fatalf("a differently worded answer is its own class, got %v", agreement)
	}
}

// TestMajorityBreaksTiesByLowestLeg: two classes of the same size must elect the
// same winner every run, and input order is the only stable ordering available —
// map iteration order would make the same inputs disagree with themselves.
func TestMajorityBreaksTiesByLowestLeg(t *testing.T) {
	outcomes := []FanOutOutcome{
		answered(0, "A", "left"),
		answered(1, "B", "right"),
		answered(2, "C", "right"),
		answered(3, "D", "left"),
	}
	for i := 0; i < 20; i++ {
		winner, _, err := MajorityWinner(outcomes)
		if err != nil {
			t.Fatalf("a 2-2 split still has a majority class: %v", err)
		}
		if winner != 0 {
			t.Fatalf("a tie must go to the lowest-index class, got %d", winner)
		}
	}
}

// TestMajorityWithoutAgreementIsAnError: majority is asked for to buy confidence
// from corroboration, so returning one arbitrary answer when nobody agreed would
// claim an agreement that never happened.
func TestMajorityWithoutAgreementIsAnError(t *testing.T) {
	_, _, err := MajorityWinner([]FanOutOutcome{
		answered(0, "A", "one"),
		answered(1, "B", "two"),
		answered(2, "C", "three"),
	})
	if err == nil {
		t.Fatal("three different answers must not elect a winner")
	}
	if !strings.Contains(err.Error(), "no two of the 3 successful subagent tasks") {
		t.Fatalf("the error must say how many answers were compared: %v", err)
	}
}

// TestMajorityCountsOnlyLegsThatAnswered: a failed or skipped leg cast no vote,
// so it can neither join a class nor make one big enough to win.
func TestMajorityCountsOnlyLegsThatAnswered(t *testing.T) {
	outcomes := []FanOutOutcome{
		answered(0, "A", "same"),
		{Index: 1, Target: "explore", Error: "provider unavailable", Reply: "same"},
		{Index: 2, Target: "explore", Skipped: true, Reply: "same"},
	}
	if _, _, err := MajorityWinner(outcomes); err == nil {
		t.Fatal("only one leg answered, so there is no majority")
	}
	outcomes = append(outcomes, answered(3, "D", "same"))
	winner, agreement, err := MajorityWinner(outcomes)
	if err != nil {
		t.Fatalf("two legs now answered the same: %v", err)
	}
	if winner != 0 || agreement[0] != 2 || agreement[3] != 2 {
		t.Fatalf("winner=%d agreement=%v; only answering legs may count", winner, agreement)
	}
	if agreement[1] != 0 || agreement[2] != 0 {
		t.Fatalf("a failed or skipped leg must have no agreement count, got %v", agreement)
	}
}

// TestReviewerPromptNumbersCandidatesByLegNumber: the reviewer's verdict and the
// fan-out the caller reads must agree on what "[2]" means, so candidates carry
// their leg number and legs that produced nothing are left out.
func TestReviewerPromptNumbersCandidatesByLegNumber(t *testing.T) {
	legs := []RunAgentTask{{Task: "audit the parser"}, {Task: "audit the parser"}, {Task: "audit the parser"}}
	prompt := BuildReviewerPrompt(legs, []FanOutOutcome{
		{Index: 0, Error: "boom"},
		answered(1, "B", "candidate two body"),
		answered(2, "C", "candidate three body"),
	})
	if strings.Contains(prompt, "Candidate 1") {
		t.Fatalf("a failed leg is not a candidate: %s", prompt)
	}
	if !strings.Contains(prompt, "Candidate 2") || !strings.Contains(prompt, "Candidate 3") {
		t.Fatalf("candidates must be numbered by leg number: %s", prompt)
	}
	if strings.Count(prompt, "audit the parser") != 1 {
		t.Fatalf("a task shared by every candidate must be stated once: %s", prompt)
	}
}

// TestReviewerPromptRepeatsPerCandidateTasksWhenTheyDiffer: legs given different
// instructions cannot share one task line, or the reviewer judges answers against
// the wrong question.
func TestReviewerPromptRepeatsPerCandidateTasksWhenTheyDiffer(t *testing.T) {
	prompt := BuildReviewerPrompt(
		[]RunAgentTask{{Task: "find the bug"}, {Task: "find the leak"}},
		[]FanOutOutcome{answered(0, "A", "x"), answered(1, "B", "y")},
	)
	if !strings.Contains(prompt, "Its task: find the bug") || !strings.Contains(prompt, "Its task: find the leak") {
		t.Fatalf("differing tasks must be stated per candidate: %s", prompt)
	}
}

// TestReviewerPromptMarksTruncation: a candidate too big for the judging budget is
// cut, and the cut is visible — a reviewer that cannot tell it read a prefix would
// judge a truncated answer as an incomplete one.
func TestReviewerPromptMarksTruncation(t *testing.T) {
	long := strings.Repeat("x", maxReviewerCandidateChars+500)
	prompt := BuildReviewerPrompt(
		[]RunAgentTask{{Task: "t"}, {Task: "t"}},
		[]FanOutOutcome{answered(0, "A", long), answered(1, "B", "short")},
	)
	if !strings.Contains(prompt, "truncated after") {
		t.Fatalf("truncation must be announced in the prompt: %s", prompt[:200])
	}
	if strings.Contains(prompt, strings.Repeat("x", maxReviewerCandidateChars+1)) {
		t.Fatal("the candidate must actually be shortened, not just labelled")
	}
}

// TestParseReviewerChoice: a verdict that cannot be read is an error, never a
// fallback pick — an arbitrary choice is the one thing the extra reviewer run was
// bought to avoid.
func TestParseReviewerChoice(t *testing.T) {
	outcomes := []FanOutOutcome{
		answered(0, "A", "a"),
		answered(1, "B", "b"),
		{Index: 2, Error: "boom"},
	}
	cases := []struct {
		name    string
		reply   string
		want    int
		wantErr string
	}{
		{name: "bare number", reply: "2", want: 1},
		{name: "number in a sentence", reply: "Candidate 1 is the best.", want: 0},
		{name: "no number at all", reply: "the second one", wantErr: "names no candidate number"},
		{name: "out of range", reply: "9", wantErr: "only 3 tasks were run"},
		{name: "picked a failed leg", reply: "3", wantErr: "produced no answer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseReviewerChoice(tc.reply, outcomes)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected an error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected leg %d, got %d", tc.want, got)
			}
		})
	}
}

// TestRankAndPickPreconditions: both new strategies compare alternatives, so one
// task cannot be compared with anything; and majority compares replies as text, so
// unconstrained free-text legs would practically never agree. Both are refused
// before any subagent runs rather than after N of them have been paid for.
func TestRankAndPickPreconditions(t *testing.T) {
	cases := []struct {
		name string
		in   runSubagentInput
		want string
	}{
		{
			name: "majority with one task",
			in: runSubagentInput{Target: "explore", Strategy: StrategyMajority, OutputFormat: "one word",
				Tasks: []fanOutTaskInput{{Task: "x"}}},
			want: "at least 2 tasks",
		},
		{
			name: "reviewer-selects with one task",
			in: runSubagentInput{Target: "explore", Strategy: StrategyReviewerSelects,
				Tasks: []fanOutTaskInput{{Task: "x"}}},
			want: "at least 2 tasks",
		},
		{
			name: "majority without an output format",
			in: runSubagentInput{Target: "explore", Strategy: StrategyMajority,
				Tasks: []fanOutTaskInput{{Task: "x"}, {Task: "y"}}},
			want: "needs an \"output_format\"",
		},
		{
			name: "majority with an output format on only one leg",
			in: runSubagentInput{Target: "explore", Strategy: StrategyMajority,
				Tasks: []fanOutTaskInput{{Task: "x", OutputFormat: "one word"}, {Task: "y"}}},
			want: "tasks[1]",
		},
		{
			name: "still-unknown strategy",
			in: runSubagentInput{Target: "explore", Strategy: "consensus",
				Tasks: []fanOutTaskInput{{Task: "x"}, {Task: "y"}}},
			want: "must be one of all, first-success, majority, reviewer-selects",
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

// TestRankAndPickAcceptsAWellFormedCall: the preconditions must not reject the
// shape the strategies exist for — one shared output_format inherited by every
// leg.
func TestRankAndPickAcceptsAWellFormedCall(t *testing.T) {
	spec, err := callFanOut(t, runSubagentInput{
		Target: "explore", Strategy: StrategyMajority, OutputFormat: "one word: yes or no",
		Tasks: []fanOutTaskInput{{Task: "x"}, {Task: "y"}, {Task: "z"}},
	})
	if err != nil {
		t.Fatalf("a shared output_format must satisfy every leg: %v", err)
	}
	if spec.Strategy != StrategyMajority || len(spec.Tasks) != 3 {
		t.Fatalf("unexpected spec %+v", spec)
	}
	if _, err := callFanOut(t, runSubagentInput{
		Target: "explore", Strategy: StrategyReviewerSelects,
		Tasks: []fanOutTaskInput{{Task: "x"}, {Task: "y"}},
	}); err != nil {
		t.Fatalf("reviewer-selects needs no output_format — a judge reads prose: %v", err)
	}
}

// TestFormatMajorityShowsOnlyTheWinnersReply: the point of majority is turning N
// answers into one, so pasting the losers back would return the pile the caller
// delegated to avoid — but every leg must still be accounted for, and a dissenter
// must be visibly a dissenter.
func TestFormatMajorityShowsOnlyTheWinnersReply(t *testing.T) {
	out := formatFanOut(RunAgentResult{
		Strategy: StrategyMajority,
		FanOut: []FanOutOutcome{
			{Index: 0, Target: "explore", AgentName: "A", Reply: "yes", Winner: true, Agreement: 2},
			{Index: 1, Target: "explore", AgentName: "B", Reply: "yes", Agreement: 2,
				Artifacts: []SubagentArtifact{{ID: "ART7", Title: "Notes", Kind: "markdown"}}},
			{Index: 2, Target: "explore", AgentName: "C", Reply: "no", Agreement: 1},
		},
	})
	if !strings.Contains(out, "winner: [1], agreed on by 2 of 3 tasks") {
		t.Fatalf("the header must state the verdict and its denominator: %s", out)
	}
	if !strings.Contains(out, "[1] A — MAJORITY ANSWER:\nyes") {
		t.Fatalf("the winner's reply must be printed in full: %s", out)
	}
	if !strings.Contains(out, "[2] B — agreed with the majority") || !strings.Contains(out, "[3] C — DISSENTED") {
		t.Fatalf("every other leg must be accounted for by name: %s", out)
	}
	if strings.Contains(out, ":\nno") {
		t.Fatalf("a losing leg's reply must not be pasted back: %s", out)
	}
	if !strings.Contains(out, "ART7") {
		t.Fatalf("a losing leg's artifacts still exist and must be listed: %s", out)
	}
}

// TestFormatReviewerSelectsNamesTheJudge: the caller must be able to tell a
// reviewer's pick from a majority vote — they carry very different confidence.
func TestFormatReviewerSelectsNamesTheJudge(t *testing.T) {
	out := formatFanOut(RunAgentResult{
		Strategy: StrategyReviewerSelects,
		FanOut: []FanOutOutcome{
			{Index: 0, Target: "explore", AgentName: "A", Reply: "first"},
			{Index: 1, Target: "explore", AgentName: "B", Reply: "second", Winner: true},
		},
	})
	if !strings.Contains(out, "winner: [2], chosen by the reviewer") {
		t.Fatalf("the header must name who chose: %s", out)
	}
	if !strings.Contains(out, "[2] B — SELECTED by the reviewer:\nsecond") {
		t.Fatalf("the selected reply must be printed in full: %s", out)
	}
	if !strings.Contains(out, "[1] A — not selected") || strings.Contains(out, "\nfirst") {
		t.Fatalf("an unselected leg is listed but not pasted back: %s", out)
	}
}

// TestCollectingStrategyRenderingIsUnchanged pins the EXACT text "all" and
// "first-success" produce. Adding rank-and-pick strategies must not move a byte of
// what existing callers already read; a golden string is the only way to prove
// that, since a substring assertion would pass through spacing or wording drift.
func TestCollectingStrategyRenderingIsUnchanged(t *testing.T) {
	firstSuccess := formatFanOut(RunAgentResult{
		Strategy: StrategyFirstSuccess,
		FanOut: []FanOutOutcome{
			{Index: 0, Target: "explore", Error: "provider unavailable"},
			{Index: 1, Target: "reviewer", AgentName: "Reviewer", Reply: "found it",
				Artifacts: []SubagentArtifact{{ID: "ART3", Title: "Notes", Kind: "markdown"}}},
			{Index: 2, Target: "coder", Skipped: true},
		},
	})
	wantFirstSuccess := "Fan-out of 3 subagent tasks (strategy: first-success)\n\n" +
		"--- [1] explore — FAILED: provider unavailable\n\n" +
		"--- [2] Reviewer:\nfound it\n" +
		"  artifact ART3 — Notes (markdown)\n\n" +
		"--- [3] coder — SKIPPED (another task already succeeded)"
	if firstSuccess != wantFirstSuccess {
		t.Fatalf("first-success rendering changed.\n got: %q\nwant: %q", firstSuccess, wantFirstSuccess)
	}

	all := formatFanOut(RunAgentResult{
		Strategy: StrategyAll,
		FanOut: []FanOutOutcome{
			{Index: 0, Target: "explore", AgentName: "Explorer", Reply: "one"},
			{Index: 1, Target: "coder", AgentName: "Coder", Reply: "two"},
		},
	})
	wantAll := "Fan-out of 2 subagent tasks (strategy: all)\n\n" +
		"--- [1] Explorer:\none\n\n" +
		"--- [2] Coder:\ntwo"
	if all != wantAll {
		t.Fatalf("all rendering changed.\n got: %q\nwant: %q", all, wantAll)
	}
}

// TestCollectingStrategiesGainNoPreconditions: the new validation must be inert on
// the old strategies — a single-task "all" fan-out with no output_format is still
// a legal call.
func TestCollectingStrategiesGainNoPreconditions(t *testing.T) {
	for _, strategy := range []string{"", StrategyAll, StrategyFirstSuccess} {
		spec, err := callFanOut(t, runSubagentInput{
			Target: "explore", Strategy: strategy,
			Tasks: []fanOutTaskInput{{Task: "only one"}},
		})
		if err != nil {
			t.Fatalf("strategy %q must still accept a single unconstrained task: %v", strategy, err)
		}
		want := strategy
		if want == "" {
			want = StrategyAll
		}
		if spec.Strategy != want {
			t.Fatalf("strategy %q resolved to %q", strategy, spec.Strategy)
		}
	}
}

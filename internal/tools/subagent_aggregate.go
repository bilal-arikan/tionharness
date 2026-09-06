package tools

import (
	"fmt"
	"strconv"
	"strings"
)

// Rank-and-pick fan-out strategies.
//
// Unlike "all" and "first-success", these two do not merely collect the legs —
// they decide WHICH leg is the answer. That needs a written-down definition of
// "the same answer" and of "no winner", because both are judgement calls that
// silently guessing would get wrong. The definitions this package settled on are
// recorded on each constant and on the functions that implement them.
const (
	// StrategyMajority runs the same question down several alternative routes and
	// returns the answer the most legs agreed on.
	//
	// DESIGN — what counts as "the same answer": replies are compared as
	// NORMALISED TEXT (trimmed, lower-cased, internal whitespace collapsed), never
	// semantically. A judging model turn would compare meaning, but it costs
	// another unbounded call and makes the verdict non-deterministic — that is
	// exactly what StrategyReviewerSelects is for, and having both would make this
	// one pointless. Normalised equality is cheap, exact and reproducible; its
	// price is that it only works on SHORT, constrained answers, which is why this
	// strategy REQUIRES output_format on every leg (validateAggregateStrategy).
	// Mandating a JSON schema for the reply was the alternative and was rejected
	// as more machinery than the strategy is worth: instructing the legs to
	// "answer with one word" buys the same comparability for one sentence.
	StrategyMajority = "majority"

	// StrategyReviewerSelects runs every candidate and has a separate reviewer
	// subagent pick one of them.
	//
	// DESIGN — who reviews: always the built-in read-only "reviewer" profile,
	// with no caller-chosen alternative. Choosing the best of N answers to one
	// question is a generic act, and a judge-selection knob is API surface no
	// caller asked for; anyone who needs a domain expert as judge can run "all"
	// and judge inside their own turn.
	StrategyReviewerSelects = "reviewer-selects"
)

// knownStrategies lists every accepted "strategy" value, in schema order.
var knownStrategies = []string{StrategyAll, StrategyFirstSuccess, StrategyMajority, StrategyReviewerSelects}

// isRankAndPick reports whether a strategy elects one winning leg.
func isRankAndPick(strategy string) bool {
	return strategy == StrategyMajority || strategy == StrategyReviewerSelects
}

// validateStrategy refuses an out-of-enum strategy instead of defaulting it: a
// misspelled "first_success" that silently ran as "all" would spend every leg the
// caller expected to be cancelled.
func validateStrategy(strategy string) error {
	for _, s := range knownStrategies {
		if s == strategy {
			return nil
		}
	}
	return fmt.Errorf("\"strategy\" must be one of %s; got %q", strings.Join(knownStrategies, ", "), strategy)
}

// validateAggregateStrategy checks the preconditions a rank-and-pick strategy
// needs in order to mean anything. Both checks are refusals at call time rather
// than failures after N subagents have already run and been paid for.
func validateAggregateStrategy(strategy string, legs []RunAgentTask) error {
	if !isRankAndPick(strategy) {
		return nil
	}
	if len(legs) < 2 {
		return fmt.Errorf("\"strategy\": %q compares alternative answers against each other, so it needs at least 2 tasks; got %d", strategy, len(legs))
	}
	if strategy != StrategyMajority {
		return nil
	}
	for i, leg := range legs {
		if leg.OutputFormat == "" {
			return fmt.Errorf("tasks[%d]: %q compares the replies as text, so every task needs an \"output_format\" that constrains the answer (e.g. \"one word: yes or no\") — set one per task or a shared one at the top level", i, StrategyMajority)
		}
	}
	return nil
}

// normalizeReply is the equality key for StrategyMajority: case-folded, with
// every run of whitespace collapsed to a single space. It removes formatting
// noise, not wording — "yes" and "Yes, definitely" remain different answers, and
// that is intended: this strategy corroborates identical answers, it does not
// paraphrase.
func normalizeReply(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// MajorityWinner groups the successful legs by normalised reply and returns the
// winning leg's index together with each leg's agreement count (the size of its
// equality class; 0 for a leg that failed or was skipped and so never voted).
//
// DESIGN — breaking a tie: the largest class wins, and between classes of equal
// size the one whose FIRST member has the lowest input index wins. The caller
// numbered the legs and that ordering is the only stable signal available;
// letting map iteration decide would make identical inputs elect different
// winners from run to run.
//
// DESIGN — when no majority forms: if the largest class has a single member
// (every leg answered differently, or only one leg survived) this is an ERROR,
// not "the biggest class wins anyway". The caller reached for majority precisely
// to buy confidence from corroboration; returning one arbitrary answer labelled
// "majority" would assert agreement that never happened.
func MajorityWinner(outcomes []FanOutOutcome) (int, []int, error) {
	type class struct {
		first int // lowest input index in this class — the tie-break key
		n     int
	}
	classes := map[string]*class{}
	keys := make([]string, len(outcomes))
	voted := make([]bool, len(outcomes))
	candidates := 0
	for i, o := range outcomes {
		if o.Error != "" || o.Skipped {
			continue
		}
		candidates++
		voted[i] = true
		key := normalizeReply(o.Reply)
		keys[i] = key
		if c, ok := classes[key]; ok {
			c.n++
			continue
		}
		classes[key] = &class{first: i, n: 1}
	}

	var win *class
	for _, c := range classes {
		if win == nil || c.n > win.n || (c.n == win.n && c.first < win.first) {
			win = c
		}
	}
	if win == nil {
		// The dispatcher refuses an all-failed fan-out before it gets here, so this
		// is unreachable — but it still returns an error rather than index 0, which
		// would silently elect a leg that never produced an answer.
		return 0, nil, fmt.Errorf("strategy %q: no subagent task produced an answer to compare", StrategyMajority)
	}
	if win.n < 2 {
		return 0, nil, fmt.Errorf(
			"strategy %q: no two of the %d successful subagent tasks gave the same answer (compared as text, ignoring case and whitespace), so there is no majority to report — re-run with strategy %q to read the divergent answers, or give the tasks a stricter \"output_format\"",
			StrategyMajority, candidates, StrategyAll)
	}

	agreement := make([]int, len(outcomes))
	for i := range outcomes {
		if voted[i] {
			agreement[i] = classes[keys[i]].n
		}
	}
	return win.first, agreement, nil
}

// maxReviewerCandidateChars caps how much of ONE candidate reply is shown to the
// reviewer in StrategyReviewerSelects.
//
// DESIGN — candidates that do not fit the reviewer's context: the judging copy is
// truncated behind a VISIBLE marker and the call carries on; it is not failed and
// nothing is dropped quietly. The reviewer's context window is not knowable from
// here, so a fixed per-candidate budget is the only honest bound, and a prefix is
// enough to tell candidates apart in practice. The truncation never reaches the
// caller either: the winner is handed back as its full original reply, not as the
// shortened judging copy.
const maxReviewerCandidateChars = 4000

// BuildReviewerPrompt renders the judging turn's instruction: the candidates,
// numbered by the LEG number the caller already sees in the fan-out output, so
// the reviewer's verdict and the rendered result talk about the same "[2]".
// Failed and skipped legs are left out — they produced nothing to judge.
func BuildReviewerPrompt(legs []RunAgentTask, outcomes []FanOutOutcome) string {
	var b strings.Builder
	b.WriteString("Several agents were given the same job and could not see each other's work. ")
	b.WriteString("Judge their answers and pick the single best one.\n\n")

	shared := sharedLegTask(legs, outcomes)
	if shared != "" {
		fmt.Fprintf(&b, "The task every candidate was given:\n%s\n\n", shared)
	}
	for i, o := range outcomes {
		if o.Error != "" || o.Skipped {
			continue
		}
		fmt.Fprintf(&b, "--- Candidate %d ---\n", i+1)
		if shared == "" {
			if task := legTask(legs, i); task != "" {
				fmt.Fprintf(&b, "Its task: %s\n", task)
			}
		}
		b.WriteString(truncateForReview(o.Reply))
		b.WriteString("\n\n")
	}
	b.WriteString("Reply with ONLY the number of the best candidate — just the digits, nothing else.")
	return b.String()
}

// sharedLegTask returns the task text when every candidate leg was given the same
// one, so the prompt states it once instead of repeating a long instruction per
// candidate — the reviewer's context is the scarce resource here.
func sharedLegTask(legs []RunAgentTask, outcomes []FanOutOutcome) string {
	shared := ""
	for i, o := range outcomes {
		if o.Error != "" || o.Skipped {
			continue
		}
		task := legTask(legs, i)
		if task == "" {
			return ""
		}
		if shared == "" {
			shared = task
			continue
		}
		if task != shared {
			return ""
		}
	}
	return shared
}

// legTask reads leg i's instruction, tolerating a caller that did not pass the
// legs alongside the outcomes.
func legTask(legs []RunAgentTask, i int) string {
	if i < 0 || i >= len(legs) {
		return ""
	}
	return legs[i].Task
}

// truncateForReview shortens one candidate to the judging budget, marking the cut
// so the reviewer knows it is looking at a prefix rather than a complete answer.
func truncateForReview(reply string) string {
	r := []rune(reply)
	if len(r) <= maxReviewerCandidateChars {
		return reply
	}
	return string(r[:maxReviewerCandidateChars]) +
		fmt.Sprintf("\n[... truncated after %d characters; judge it on this much]", maxReviewerCandidateChars)
}

// ParseReviewerChoice reads the reviewer's verdict: the first run of digits in its
// reply, taken as a candidate (leg) number.
//
// DESIGN — an unreadable verdict is an ERROR. Falling back to "then take the
// first candidate" would turn a reviewer that misunderstood its job into a silent
// arbitrary pick — which is the single thing the caller paid a whole extra agent
// run to avoid.
func ParseReviewerChoice(reply string, outcomes []FanOutOutcome) (int, error) {
	digits := firstNumber(reply)
	if digits == "" {
		return 0, fmt.Errorf("the reviewer's verdict names no candidate number: %q", truncateForError(reply))
	}
	n, err := strconv.Atoi(digits)
	if err != nil {
		return 0, fmt.Errorf("the reviewer's verdict %q is not a usable candidate number: %w", digits, err)
	}
	idx := n - 1
	if idx < 0 || idx >= len(outcomes) {
		return 0, fmt.Errorf("the reviewer picked candidate %d, but only %d tasks were run", n, len(outcomes))
	}
	if o := outcomes[idx]; o.Error != "" || o.Skipped {
		return 0, fmt.Errorf("the reviewer picked candidate %d, which produced no answer to pick", n)
	}
	return idx, nil
}

// firstNumber returns the first run of digits in s, or "" when there is none.
func firstNumber(s string) string {
	start := -1
	for i, r := range s {
		if r >= '0' && r <= '9' {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			return s[start:i]
		}
	}
	if start >= 0 {
		return s[start:]
	}
	return ""
}

// truncateForError keeps a quoted reply short enough to read in an error message.
func truncateForError(s string) string {
	const max = 200
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max]) + "…"
}

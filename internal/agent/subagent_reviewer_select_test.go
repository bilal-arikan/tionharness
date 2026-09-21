package agent

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// reviewerSelectProvider answers both roles of a "reviewer-selects" fan-out from
// one scripted table: a candidate leg is keyed by the task it was given, and the
// judging turn is recognised by the marker BuildReviewerPrompt always writes.
//
// One provider for both roles is what the production path actually does — the
// ephemeral reviewer clones the caller's provider (ephemeralSubagent) — so
// splitting them in the fixture would test a wiring that does not exist.
type reviewerSelectProvider struct {
	// byTask maps a leg's task text to the reply that leg returns.
	byTask map[string]string
	// verdict is the judging turn's reply, read as a candidate NUMBER (1-based).
	verdict string
	// reviewerPrompts records every judging prompt, so a test can assert what the
	// reviewer was actually shown.
	reviewerPrompts []string
	mu              sync.Mutex
}

func (*reviewerSelectProvider) Name() string { return "reviewer-select-test" }

func (p *reviewerSelectProvider) Complete(_ context.Context, req providers.Request) (*providers.Response, error) {
	prompt := lastUserText(req)
	p.mu.Lock()
	defer p.mu.Unlock()
	// The judging turn is the only one carrying BuildReviewerPrompt's header.
	if strings.Contains(prompt, "Judge their answers and pick the single best one.") {
		p.reviewerPrompts = append(p.reviewerPrompts, prompt)
		return &providers.Response{StopReason: providers.StopEndTurn, Text: p.verdict}, nil
	}
	reply, ok := p.byTask[strings.TrimSpace(prompt)]
	if !ok {
		// Silence here would make an unrouted leg look like an empty answer and the
		// assertions would then chase the wrong bug.
		return nil, errUnroutedTask
	}
	return &providers.Response{StopReason: providers.StopEndTurn, Text: reply}, nil
}

// errUnroutedTask marks a fixture miss: a leg reached the provider with a task
// the test never scripted.
var errUnroutedTask = errorString("reviewer-select fixture: unscripted task reached the provider")

type errorString string

func (e errorString) Error() string { return string(e) }

// lastUserText returns the final user message, which is the task (isolated
// context) or the judging prompt.
func lastUserText(req providers.Request) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == providers.RoleUser {
			return req.Messages[i].Text
		}
	}
	return ""
}

var registerReviewerSelectProvider sync.Once

// configureReviewerSelect points the runtime (and therefore every ephemeral
// subagent cloned from the caller) at the scripted provider.
func configureReviewerSelect(rt *Runtime, p *reviewerSelectProvider) {
	registerReviewerSelectProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "reviewer-select-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return activeReviewerSelect.Load(), nil },
		))
	})
	activeReviewerSelect.Store(p)
	rt.providers.SetInstances([]providers.Instance{{ID: "reviewer-select-test", KindID: "reviewer-select-test"}})
}

// activeReviewerSelect is the provider the registered kind hands out. The kind is
// registered process-wide exactly once, so the per-test script is swapped here
// rather than captured in the constructor.
var activeReviewerSelect atomic.Pointer[reviewerSelectProvider]

// reviewerSelectCaller creates the delegating agent plus the session and
// delegation state a subagent run requires, and returns the ready context.
func reviewerSelectCaller(t *testing.T, rt *Runtime) (db.Agent, context.Context) {
	t.Helper()
	ctx := context.Background()
	caller, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Caller", Provider: "reviewer-select-test", Model: "test",
	})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	session, err := rt.db.CreateSession(ctx, db.Session{AgentID: caller.ID, Title: "fan-out"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	var calls int32
	ctx = context.WithValue(ctx, delegStateKey{}, delegState{
		depth: 0, visited: map[string]bool{caller.ID: true}, calls: &calls,
	})
	return caller, WithSessionID(ctx, session.ID)
}

// renderFanOut formats a finished fan-out the way the tool hands it to the
// calling model, which is where the "winner in full, losers silent" contract is
// actually observable.
func renderFanOut(t *testing.T, res tools.RunAgentResult) string {
	t.Helper()
	rendered, err := tools.FormatRunAgentResult(res)
	if err != nil {
		t.Fatalf("format fan-out result: %v", err)
	}
	return rendered
}

// threeCandidateSpec is the fan-out under test: three legs, same question, each
// answering differently so the winner is identifiable by its text alone.
func threeCandidateSpec() tools.RunAgentSpec {
	return tools.RunAgentSpec{
		Strategy: tools.StrategyReviewerSelects,
		Tasks: []tools.RunAgentTask{
			{Target: "explore", Task: "route A"},
			{Target: "explore", Task: "route B"},
			{Target: "explore", Task: "route C"},
		},
	}
}

// candidateReplies are deliberately distinct, hardcoded strings: an assertion
// that recomputed the expectation from the outcomes would pass against a bug
// that returns the wrong leg.
var candidateReplies = map[string]string{
	"route A": "ANSWER-FROM-A: cache the index",
	"route B": "ANSWER-FROM-B: shard the writes",
	"route C": "ANSWER-FROM-C: drop the join",
}

// TestReviewerSelectsReturnsTheJudgesPickNotTheFirstLeg is the end-to-end guard
// this strategy was missing: the reviewer names candidate 3, so leg 3 must be the
// one elected and the one whose reply is printed in full. Picking the LAST of
// three catches the whole family of "elect index 0" / "elect the first that
// answered" bugs, which a two-leg fixture or a first-candidate verdict would let
// through.
func TestReviewerSelectsReturnsTheJudgesPickNotTheFirstLeg(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &reviewerSelectProvider{byTask: candidateReplies, verdict: "3"}
	configureReviewerSelect(rt, p)
	caller, ctx := reviewerSelectCaller(t, rt)

	res, err := rt.runAgentFanOut(ctx, caller, nil, true, threeCandidateSpec())
	if err != nil {
		t.Fatalf("fan-out with a working reviewer must succeed: %v", err)
	}

	if len(res.FanOut) != 3 {
		t.Fatalf("every leg must be reported, got %d", len(res.FanOut))
	}
	if res.FanOut[0].Winner || res.FanOut[1].Winner || !res.FanOut[2].Winner {
		t.Fatalf("the reviewer named candidate 3; exactly leg 3 must be the winner, got %+v", res.FanOut)
	}
	// Hardcoded, not res.FanOut[2].Reply: the point is that the JUDGE's choice
	// selected this specific text.
	if got := res.FanOut[2].Reply; got != "ANSWER-FROM-C: drop the join" {
		t.Fatalf("the winning leg must carry candidate 3's own answer, got %q", got)
	}

	// The rendered result is what the calling model actually reads, so the contract
	// is asserted there too: the winner in full, the losers named but silent.
	rendered := renderFanOut(t, res)
	if !strings.Contains(rendered, "ANSWER-FROM-C: drop the join") {
		t.Fatalf("the winner's reply must be printed in full:\n%s", rendered)
	}
	if strings.Contains(rendered, "ANSWER-FROM-A: cache the index") ||
		strings.Contains(rendered, "ANSWER-FROM-B: shard the writes") {
		t.Fatalf("a losing candidate's reply must not be pasted back:\n%s", rendered)
	}
	if !strings.Contains(rendered, "winner: [3], chosen by the reviewer") {
		t.Fatalf("the verdict line must name the reviewer's pick:\n%s", rendered)
	}
	// The losers disappear entirely only if the report drops them — they must stay
	// listed with a reason.
	if strings.Count(rendered, "not selected") != 2 {
		t.Fatalf("both losing legs must still be listed as not selected:\n%s", rendered)
	}

	// The judge must have been shown the candidates it was asked to rank; a reviewer
	// that picked "3" without seeing three answers would pass every check above.
	if len(p.reviewerPrompts) != 1 {
		t.Fatalf("exactly one judging turn must run, got %d", len(p.reviewerPrompts))
	}
	for _, want := range []string{"Candidate 1", "Candidate 2", "Candidate 3", "ANSWER-FROM-C: drop the join"} {
		if !strings.Contains(p.reviewerPrompts[0], want) {
			t.Fatalf("the judging prompt must contain %q:\n%s", want, p.reviewerPrompts[0])
		}
	}
	drainSpawns(t, rt)
}

// TestReviewerSelectsFollowsADifferentVerdict runs the identical fan-out with the
// only change being the judge's answer. If the winner moves with it, selection is
// genuinely driven by the reviewer rather than by leg order or reply content.
func TestReviewerSelectsFollowsADifferentVerdict(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &reviewerSelectProvider{byTask: candidateReplies, verdict: "2"}
	configureReviewerSelect(rt, p)
	caller, ctx := reviewerSelectCaller(t, rt)

	res, err := rt.runAgentFanOut(ctx, caller, nil, true, threeCandidateSpec())
	if err != nil {
		t.Fatalf("fan-out with a working reviewer must succeed: %v", err)
	}

	if res.FanOut[0].Winner || !res.FanOut[1].Winner || res.FanOut[2].Winner {
		t.Fatalf("the reviewer named candidate 2; exactly leg 2 must be the winner, got %+v", res.FanOut)
	}
	if got := res.FanOut[1].Reply; got != "ANSWER-FROM-B: shard the writes" {
		t.Fatalf("the winning leg must carry candidate 2's own answer, got %q", got)
	}

	rendered := renderFanOut(t, res)
	if !strings.Contains(rendered, "ANSWER-FROM-B: shard the writes") {
		t.Fatalf("the winner's reply must be printed in full:\n%s", rendered)
	}
	if strings.Contains(rendered, "ANSWER-FROM-C: drop the join") {
		t.Fatalf("candidate 3 lost this time; its reply must not be pasted back:\n%s", rendered)
	}
	if !strings.Contains(rendered, "winner: [2], chosen by the reviewer") {
		t.Fatalf("the verdict line must name the reviewer's pick:\n%s", rendered)
	}
	drainSpawns(t, rt)
}

// TestReviewerSelectsRefusesAVerdictNamingAMissingCandidate: the judge answering
// "4" for a three-leg fan-out is a misunderstanding, and quietly falling back to
// any leg would be the silent arbitrary pick the extra reviewer run was paid to
// avoid. The call must fail and elect nobody.
func TestReviewerSelectsRefusesAVerdictNamingAMissingCandidate(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	p := &reviewerSelectProvider{byTask: candidateReplies, verdict: "4"}
	configureReviewerSelect(rt, p)
	caller, ctx := reviewerSelectCaller(t, rt)

	_, err := rt.runAgentFanOut(ctx, caller, nil, true, threeCandidateSpec())
	if err == nil {
		t.Fatal("a verdict naming a candidate that does not exist must fail the call")
	}
	if !strings.Contains(err.Error(), "picked candidate 4") {
		t.Fatalf("the error must name the unusable verdict, got %v", err)
	}
	drainSpawns(t, rt)
}

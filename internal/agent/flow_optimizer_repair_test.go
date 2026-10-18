package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// lookaheadProposal routes with a Perl lookahead, which Go's RE2 rejects — the
// slip the observer made run after run on a real workspace.
const lookaheadProposal = `{"noChange":false,"reason":"3/3 runs passed unverified","confidence":0.9,"ops":[{"op":"insert_between","from":"respond","to":"output","node":{"id":"verify","type":"route","mode":"regex"}},{"op":"update_edge","id":"e_verify_output","edge":{"when":"^(?!.*FAIL).*PASS"}}]}`

// repairFixture runs one turn on a fresh agent and answers the observer with
// first, then (on the repair follow-up) with second; it records whether the
// follow-up carried the validator's error.
func repairFixture(t *testing.T, first, second string) (*Runtime, db.Flow, *string) {
	t.Helper()
	rt, _ := newTestRuntime(t, t.TempDir())
	var repairAsk string
	p := &flowTestProvider{}
	p.reply = func(call int, req providers.Request) (*providers.Response, error) {
		u := lastUser(req)
		switch {
		case strings.Contains(u, "rejected by the flow validator"):
			repairAsk = u
			return &providers.Response{Text: second, StopReason: providers.StopEndTurn}, nil
		case strings.Contains(u, "# Flow"):
			return &providers.Response{Text: first, StopReason: providers.StopEndTurn}, nil
		}
		return &providers.Response{Text: "ok", StopReason: providers.StopEndTurn}, nil
	}
	installFlowTestProvider(t, rt, p)
	a := flowTestAgent(t, rt, "R")
	f, err := rt.db.EnsureAgentFlow(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTurn(t, rt, a, "hello"); err != nil {
		t.Fatal(err)
	}
	return rt, f, &repairAsk
}

// TestFlowObserverRepairsRejectedProposal: an invalid first answer is sent back
// with the validator's error once; a valid second answer is the one filed.
func TestFlowObserverRepairsRejectedProposal(t *testing.T) {
	rt, f, repairAsk := repairFixture(t, lookaheadProposal, autoAcceptProposal)
	res, err := rt.OptimizeFlow(context.Background(), f.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(*repairAsk, "invalid") {
		t.Fatalf("repair follow-up must quote the validator error, got %q", *repairAsk)
	}
	if res.Proposal == nil || res.Proposal.Status != db.ProposalPending || res.Proposal.Error != "" {
		t.Fatalf("repaired proposal must be filed pending, got %+v", res.Proposal)
	}
	if !strings.Contains(string(res.Proposal.Ops), `"critic"`) {
		t.Fatalf("filed ops must be the repaired ones, got %s", res.Proposal.Ops)
	}
}

// TestFlowObserverRepairStillInvalid: a second invalid answer files the
// proposal as invalid with both errors, and no third call is made.
func TestFlowObserverRepairStillInvalid(t *testing.T) {
	rt, f, _ := repairFixture(t, lookaheadProposal, `{"noChange":false,"reason":"x","confidence":0.9,"ops":[{"op":"remove_node","id":"output"}]}`)
	res, err := rt.OptimizeFlow(context.Background(), f.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposal == nil || res.Proposal.Status != db.ProposalInvalid || !strings.Contains(res.Proposal.Error, "after repair") {
		t.Fatalf("still-invalid proposal = %+v", res.Proposal)
	}
	if !strings.Contains(string(res.Proposal.Ops), "verify") {
		t.Fatalf("the invalid record keeps the first attempt's ops, got %s", res.Proposal.Ops)
	}
}

// TestFlowObserverBacksOffAfterQuietPasses: every pass that changed nothing
// doubles the runs the next automatic look waits for (capped); a filed
// proposal resets the streak.
func TestFlowObserverBacksOffAfterQuietPasses(t *testing.T) {
	if got := []int{flowObserverDueRuns(5, 0), flowObserverDueRuns(5, 1), flowObserverDueRuns(5, 2), flowObserverDueRuns(5, 9)}; got[0] != 5 || got[1] != 10 || got[2] != 20 || got[3] != 40 {
		t.Fatalf("due runs = %v, want [5 10 20 40]", got)
	}
	answer := `{"noChange":true,"reason":"same environment problem","confidence":0.9}`
	rt, _ := newTestRuntime(t, t.TempDir())
	_, f := autoAcceptFixture(t, rt, func() string { return answer })
	ctx := context.Background()
	for want := 1; want <= 2; want++ {
		if _, err := rt.OptimizeFlow(ctx, f.ID, "manual"); err != nil {
			t.Fatal(err)
		}
		if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Stats.QuietPasses != want {
			t.Fatalf("after %d quiet passes the streak is %d", want, cur.Stats.QuietPasses)
		}
	}
	answer = autoAcceptProposal
	if _, err := rt.OptimizeFlow(ctx, f.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Stats.QuietPasses != 0 {
		t.Fatalf("a filed proposal must reset the streak, got %d", cur.Stats.QuietPasses)
	}
}

// gateLoopProposal is a criteria gate after respond whose fail arm loops back
// into respond (thread context, tools).
const gateLoopProposal = `{"noChange":false,"reason":"4/8 runs claimed PASS without evidence","confidence":0.9,"ops":[{"op":"insert_between","from":"respond","to":"output","node":{"id":"verify","type":"route","mode":"criteria","criteria":["no unproven PASS"]}},{"op":"add_edge","edge":{"from":"verify","to":"respond","when":"fail"}}]}`

// TestFlowAcceptsGateAndRetryLoop: the gate-and-retry loop back into the agent's
// own turn is a valid proposal for an ordinary agent.
func TestFlowAcceptsGateAndRetryLoop(t *testing.T) {
	rt, f, _ := repairFixture(t, gateLoopProposal, gateLoopProposal)
	res, err := rt.OptimizeFlow(context.Background(), f.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if res.Proposal != nil && res.Proposal.Status == db.ProposalInvalid {
		t.Fatalf("proposal = %+v", res.Proposal)
	}
}

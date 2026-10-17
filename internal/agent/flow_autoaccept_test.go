package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
)

const autoAcceptProposal = `{"noChange":false,"reason":"add a critic","expected":"fewer errors","confidence":0.9,"ops":[{"op":"insert_between","from":"respond","to":"output","node":{"id":"critic","type":"llm","context":"fresh","tools":"none","prompt":"Review: {{node.respond}}"}}]}`

// autoAcceptFixture is an agent with one run on its default flow and an
// observer that always answers with the proposal returned by reply.
func autoAcceptFixture(t *testing.T, rt *Runtime, reply func() string) (db.Agent, db.Flow) {
	t.Helper()
	installFlowTestProvider(t, rt, okProvider(func(u string) string {
		if strings.Contains(u, "# Flow") {
			return reply()
		}
		return "ok"
	}))
	a := flowTestAgent(t, rt, "AA")
	f, err := rt.db.EnsureAgentFlow(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTurn(t, rt, a, "hello"); err != nil {
		t.Fatal(err)
	}
	return a, f
}

func setFlowMode(t *testing.T, rt *Runtime, flowID, mode string) {
	t.Helper()
	pol := db.DefaultFlowPolicy()
	pol.Mode = mode
	if _, err := rt.db.UpdateFlowMeta(context.Background(), flowID, nil, nil, &pol); err != nil {
		t.Fatal(err)
	}
}

// TestFlowAutoAcceptWorkspaceSwitch: with the workspace switch on, a propose
// flow applies a confident proposal by itself and records why; an off flow's
// manual pass still only files.
func TestFlowAutoAcceptWorkspaceSwitch(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	_, f := autoAcceptFixture(t, rt, func() string { return autoAcceptProposal })
	ctx := context.Background()
	rt.SetFlowAutoAccept(true)

	res, err := rt.OptimizeFlow(ctx, f.ID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Proposal == nil || res.Proposal.Status != db.ProposalApplied || res.Proposal.AutoApplied != db.FlowAutoAppliedWorkspace {
		t.Fatalf("workspace auto-accept result = %+v", res)
	}
	stored, _ := rt.db.GetFlowProposal(ctx, res.Proposal.ID)
	if stored.AutoApplied != db.FlowAutoAppliedWorkspace {
		t.Fatalf("autoApplied not persisted: %+v", stored)
	}
	if cur, _ := rt.db.GetFlow(ctx, f.ID); cur.Version != 2 {
		t.Fatalf("auto-accept must commit a new head, got v%d", cur.Version)
	}

	// The flow's own auto policy records "flow".
	if _, err := rt.RevertFlow(ctx, f.ID, 1, db.FlowAuthor{Kind: db.FlowAuthorUser}, ""); err != nil {
		t.Fatal(err)
	}
	setFlowMode(t, rt, f.ID, db.FlowPolicyAuto)
	res, err = rt.OptimizeFlow(ctx, f.ID, "auto")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Applied || res.Proposal.AutoApplied != db.FlowAutoAppliedFlow {
		t.Fatalf("flow auto policy result = %+v", res)
	}

	// Off stays off: the switch never turns an off flow on.
	if _, err := rt.RevertFlow(ctx, f.ID, 1, db.FlowAuthor{Kind: db.FlowAuthorUser}, ""); err != nil {
		t.Fatal(err)
	}
	setFlowMode(t, rt, f.ID, db.FlowPolicyOff)
	res, err = rt.OptimizeFlow(ctx, f.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	if res.Applied || res.Proposal == nil || res.Proposal.Status != db.ProposalPending {
		t.Fatalf("off flow result = %+v", res)
	}
	drainSpawns(t, rt)
}

// TestFlowAutoAcceptSweepsPending: proposals filed while a human had to decide
// are applied (oldest first, same road as Apply) once the switch is on; one
// below the confidence floor stays pending.
func TestFlowAutoAcceptSweepsPending(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	reply := autoAcceptProposal
	_, f := autoAcceptFixture(t, rt, func() string { return reply })
	ctx := context.Background()

	first, err := rt.OptimizeFlow(ctx, f.ID, "manual")
	if err != nil || first.Proposal == nil || first.Proposal.Status != db.ProposalPending {
		t.Fatalf("first pass = %+v %v", first, err)
	}
	reply = strings.Replace(autoAcceptProposal, `"confidence":0.9`, `"confidence":0.4`, 1)
	weak, err := rt.OptimizeFlow(ctx, f.ID, "manual")
	if err != nil || weak.Proposal == nil || weak.Proposal.Status != db.ProposalPending {
		t.Fatalf("weak pass = %+v %v", weak, err)
	}
	// Switch off: a sweep does nothing.
	if n := rt.sweepFlowProposals(ctx, ""); n != 0 {
		t.Fatalf("sweep with the switch off applied %d", n)
	}
	rt.SetFlowAutoAccept(true)
	if n := rt.sweepFlowProposals(ctx, ""); n != 1 {
		t.Fatalf("sweep applied %d, want 1", n)
	}
	got, _ := rt.db.GetFlowProposal(ctx, first.Proposal.ID)
	if got.Status != db.ProposalApplied || got.AutoApplied != db.FlowAutoAppliedWorkspace || got.AppliedVersion != 2 {
		t.Fatalf("swept proposal = %+v", got)
	}
	if left, _ := rt.db.GetFlowProposal(ctx, weak.Proposal.ID); left.Status != db.ProposalPending {
		t.Fatalf("below-floor proposal must stay pending: %+v", left)
	}
	versions, _ := rt.db.ListFlowVersions(ctx, f.ID)
	if len(versions) == 0 || versions[0].Author.Kind != db.FlowAuthorObserver {
		t.Fatalf("auto-accepted version must be authored by the observer: %+v", versions)
	}
	drainSpawns(t, rt)
}

// TestFlowAutoAcceptSweepRespectsGate: the sweep asks the proposal gate like
// the observer's own auto path; a "no" leaves the proposal pending.
func TestFlowAutoAcceptSweepRespectsGate(t *testing.T) {
	rt, tun := newTestRuntime(t, t.TempDir())
	stub := newDecisionStub(t)
	wireDecider(t, tun, stub, map[string]decider.Mode{authFlowProposalGate: decider.ModeOn})
	_, f := autoAcceptFixture(t, rt, func() string { return autoAcceptProposal })
	ctx := context.Background()
	res, err := rt.OptimizeFlow(ctx, f.ID, "manual")
	if err != nil || res.Proposal == nil {
		t.Fatalf("pass = %+v %v", res, err)
	}
	rt.SetFlowAutoAccept(true)
	stub.set(func(s *decisionStub) { s.noul[flowProposalGateKey] = 0.2 })
	if n := rt.sweepFlowProposals(ctx, f.ID); n != 0 {
		t.Fatalf("gate said no, sweep applied %d", n)
	}
	if got, _ := rt.db.GetFlowProposal(ctx, res.Proposal.ID); got.Status != db.ProposalPending {
		t.Fatalf("held proposal = %+v", got)
	}
	stub.set(func(s *decisionStub) { s.noul[flowProposalGateKey] = 0.95 })
	if n := rt.sweepFlowProposals(ctx, f.ID); n != 1 {
		t.Fatalf("gate said yes, sweep applied %d", n)
	}
	waitBackground(rt)
}

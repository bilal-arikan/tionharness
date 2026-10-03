package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/decider"
	"github.com/bilal-arikan/tionharness/internal/flow"
)

// Decision authority "flow-proposal-gate" (_Docs/93): under the auto policy a
// confident observer proposal is applied without a human. This gate gives a
// cheap, calibrated second opinion first — "is this a plausible, proportionate
// improvement given the evidence?" — the same way tool-risk second-guesses an
// unattended shell command. Off: apply as before. Shadow: apply, but record
// what the gate would have said. On: a "no" keeps the proposal pending for a
// human; a model failure falls back to applying (the previous behaviour).

const (
	flowProposalGateKey      = "improves"
	flowProposalGateOpsRunes = 6000
)

// gateFlowProposal returns whether the proposal may be auto-applied and, when
// not, a short reason for the notification.
func (r *Runtime) gateFlowProposal(ctx context.Context, caller db.Agent, f db.Flow, head flow.Graph, p db.FlowProposal) (bool, string) {
	mode := r.deciderMode(authFlowProposalGate)
	if mode == decider.ModeOff {
		return true, ""
	}
	req := decider.Request{
		State: flowProposalGateState(f, head, p),
		Questions: map[string]decider.Question{flowProposalGateKey: decider.Noul(
			"Will applying this proposal improve the flow's results for its users without breaking the flow's purpose? "+
				"Weigh the evidence from the recent runs, the size of the change and whether the expected effect is plausible.",
			"Yes: a plausible, proportionate improvement that the evidence supports.",
			"No: unsupported by the evidence, oversized, risky, or likely to harm the flow's purpose.")},
	}
	threshold := r.deciderThreshold(authFlowProposalGate)
	outcome := func(resp *decider.Response) (string, float64) {
		if resp == nil {
			return "", 0
		}
		a := resp.Answers[flowProposalGateKey]
		if a.Yes(threshold) {
			return "apply", a.Probability
		}
		return "hold", a.Probability
	}
	if mode == decider.ModeShadow {
		r.shadowDecision(ctx, authFlowProposalGate, caller, req, "apply", p.ID, outcome)
		return true, ""
	}
	resp, err := r.decide(ctx, authFlowProposalGate, caller, req, decider.WithRef(p.ID), decider.WithOutcome(outcome))
	rec := decider.NewRecord(authFlowProposalGate, decider.ModeOn, resp, err)
	rec.Ref = p.ID
	if err != nil {
		if !decisionOff(err) {
			r.logDecision(rec)
		}
		return true, ""
	}
	verdict, strength := outcome(resp)
	rec.Outcome, rec.Strength, rec.Applied = verdict, strength, true
	r.logDecision(rec)
	if verdict == "apply" {
		return true, ""
	}
	return false, fmt.Sprintf("karar mercii otomatik uygulamayı durdurdu (iyileştirme olasılığı %.0f%%)", strength*100)
}

// flowProposalGateState is the evidence the gate reads: the flow, its stats,
// the proposal and the diff it would produce.
func flowProposalGateState(f db.Flow, head flow.Graph, p db.FlowProposal) string {
	var b strings.Builder
	st := f.Stats
	fmt.Fprintf(&b, "# Flow\nname: %s\nversion: v%d\nshape: %s\n", f.Name, f.Version, head.Summary())
	if f.Note != "" {
		fmt.Fprintf(&b, "note: %s\n", f.Note)
	}
	fmt.Fprintf(&b, "\n# Stats\nruns=%d success=%d failure=%d", st.Runs, st.Success, st.Failure)
	if st.Graded > 0 {
		fmt.Fprintf(&b, " avgGrade=%.1f/5 (%d graded)", st.AvgGrade(), st.Graded)
	}
	fmt.Fprintf(&b, "\n\n# Proposal\nreason: %s\nexpected: %s\nconfidence: %.2f\nevidence: %s\n",
		p.Reason, p.Expected, p.Confidence, p.Evidence)
	if len(p.Ops) > 0 && string(p.Ops) != "null" {
		fmt.Fprintf(&b, "ops: %s\n", truncateRunes(string(p.Ops), flowProposalGateOpsRunes))
		if ops, err := flow.ParseOps(p.Ops); err == nil {
			if next, err := flow.Apply(head, ops); err == nil {
				fmt.Fprintf(&b, "resulting shape: %s\ndiff: %s\n", next.Summary(), flow.Diff(head, next).String())
			}
		}
	}
	if p.Prompt != nil {
		if p.Prompt.Soul != nil {
			fmt.Fprintf(&b, "new soul: %s\n", truncateRunes(*p.Prompt.Soul, flowOptimizerSoulRunes))
		}
		if p.Prompt.Identity != nil {
			fmt.Fprintf(&b, "new identity: %s\n", truncateRunes(*p.Prompt.Identity, flowOptimizerSoulRunes/2))
		}
	}
	return b.String()
}

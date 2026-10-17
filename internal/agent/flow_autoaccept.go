package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
)

// Auto-accept for flow evolution (_Docs/93 §3). A proposal may be applied
// without a human in two cases: the flow's own policy is auto, or the
// workspace-wide switch is on and the flow's policy is propose (off stays off).
// Either way the proposal takes the same road as a hand-applied one —
// ApplyFlowProposal — behind the same confidence floor and the
// flow-proposal-gate, and the setting that applied it is recorded on the
// proposal (FlowProposal.AutoApplied).

// SetFlowAutoAccept sets the workspace-wide auto-accept switch. It only flips
// the flag; turning it on does not sweep by itself (see SweepFlowProposals), so
// a boot-time load never re-asks the gate about proposals it already held.
func (r *Runtime) SetFlowAutoAccept(on bool) { r.flowAutoAccept.Store(on) }

// FlowAutoAccept reports the workspace-wide auto-accept switch.
func (r *Runtime) FlowAutoAccept() bool { return r.flowAutoAccept.Load() }

// flowAutoSource names the setting that lets a flow with this policy
// auto-apply proposals, or "" when a human has to.
func (r *Runtime) flowAutoSource(pol db.FlowPolicy) string {
	switch pol.Mode {
	case db.FlowPolicyAuto:
		return db.FlowAutoAppliedFlow
	case db.FlowPolicyPropose:
		if r.FlowAutoAccept() {
			return db.FlowAutoAppliedWorkspace
		}
	}
	return ""
}

// autoApplyFlowProposal runs one confident proposal through the gate and, when
// it passes, applies it exactly as the Apply button does and records the
// setting that did it. A non-empty hold means the gate kept it pending.
func (r *Runtime) autoApplyFlowProposal(ctx context.Context, caller db.Agent, f db.Flow, head flow.Graph, p db.FlowProposal, source string) (db.FlowProposal, string, error) {
	if ok, hold := r.gateFlowProposal(ctx, caller, f, head, p); !ok {
		return p, hold, nil
	}
	applied, err := r.ApplyFlowProposal(ctx, p.ID, db.FlowAuthor{Kind: db.FlowAuthorObserver, ID: caller.ID})
	if err != nil {
		return applied, "", err
	}
	marked, err := r.db.MarkFlowProposalAutoApplied(ctx, p.ID, source)
	if err != nil {
		r.logger.Warn("flow auto-accept: record failed", "proposal", p.ID, "error", err)
		return applied, "", nil
	}
	return marked, "", nil
}

// SweepFlowProposals applies, in the background, the pending proposals the
// current settings now let auto-apply: called when the workspace switch is
// turned on (flowID "") or a flow's policy moves to auto (that flow only).
// Returns false when the workspace is closing and nothing was scheduled.
func (r *Runtime) SweepFlowProposals(flowID string) bool {
	return r.startBackgroundTurn(func() {
		if n := r.sweepFlowProposals(context.Background(), flowID); n > 0 {
			r.logger.Info("flow auto-accept: pending proposals applied", "flow", flowID, "applied", n)
		}
	})
}

// sweepFlowProposals is the synchronous body of SweepFlowProposals; it
// returns how many proposals were applied.
func (r *Runtime) sweepFlowProposals(ctx context.Context, flowID string) int {
	props, err := r.db.ListFlowProposals(ctx, flowID)
	if err != nil {
		r.logger.Warn("flow auto-accept: list proposals failed", "flow", flowID, "error", err)
		return 0
	}
	// Oldest first, grouped per flow: each apply moves the head the next one
	// is re-applied onto, the same order a human working the list would use.
	var order []string
	byFlow := map[string][]db.FlowProposal{}
	for i := len(props) - 1; i >= 0; i-- {
		p := props[i]
		if p.Status != db.ProposalPending {
			continue
		}
		if _, seen := byFlow[p.FlowID]; !seen {
			order = append(order, p.FlowID)
		}
		byFlow[p.FlowID] = append(byFlow[p.FlowID], p)
	}
	applied := 0
	for _, id := range order {
		applied += r.sweepFlow(ctx, id, byFlow[id])
	}
	return applied
}

// sweepFlow auto-applies one flow's pending proposals (oldest first). It holds
// the flow's observer slot so a pass and a sweep never apply at once; a busy
// flow is skipped, its proposals stay pending for a human or the next sweep.
func (r *Runtime) sweepFlow(ctx context.Context, flowID string, pending []db.FlowProposal) int {
	if _, busy := flowOptimizerFlights.LoadOrStore(flowID, true); busy {
		r.logger.Info("flow auto-accept: observer pass running, sweep skipped", "flow", flowID)
		return 0
	}
	defer flowOptimizerFlights.Delete(flowID)
	f, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return 0
	}
	pol := f.Policy.Normalized()
	source := r.flowAutoSource(pol)
	if source == "" {
		return 0
	}
	owner, err := r.db.GetAgent(ctx, f.AgentID)
	if err != nil {
		return 0
	}
	// The observer would have applied it; when its agent cannot be resolved
	// the gate's call is billed to the flow's owner instead.
	caller := owner
	if exec, _, rerr := r.resolveAnalysisSystemAgent(flowOptimizerSystemKey, owner); rerr == nil {
		caller = exec
	}
	applied := 0
	for _, p := range pending {
		if p.Confidence < pol.MinConfidence || (p.Prompt != nil && !pol.AllowPromptChanges) {
			continue
		}
		cur, err := r.db.GetFlow(ctx, flowID)
		if err != nil {
			return applied
		}
		head, err := flow.Parse(cur.Graph)
		if err != nil {
			return applied
		}
		got, hold, err := r.autoApplyFlowProposal(ctx, caller, cur, head, p, source)
		switch {
		case hold != "":
			continue
		case err != nil:
			r.logger.Warn("flow auto-accept: apply failed", "flow", flowID, "proposal", p.ID, "error", err)
			continue
		}
		applied++
		r.emitFlowChanged(flowID, "🧬 Akış evrildi (otomatik kabul) — "+cur.Name, fmt.Sprintf("v%d: %s", got.AppliedVersion, got.Reason))
	}
	return applied
}

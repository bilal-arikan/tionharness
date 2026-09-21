package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// withRunAdhocFlow wires the run_adhoc_flow tool for one completion, bound to the
// calling agent. Mirrors withRunAgent: reqPtr is the live request so an
// inherited-context leg sees the conversation as it stands when the tool fires.
// It must run after withRunAgent, whose delegation chain position (depth,
// visited-set, shared budget counter) the legs inherit.
func (r *Runtime) withRunAdhocFlow(ctx context.Context, caller db.Agent, reqPtr *providers.Request, autonomous bool) context.Context {
	fn := func(rctx context.Context, spec tools.AdhocFlowSpec) (tools.AdhocFlowResult, error) {
		run := newAdhocRun(0, func(fctx context.Context, fspec tools.RunAgentSpec) (tools.RunAgentResult, error) {
			return r.runAgentFanOut(fctx, caller, reqPtr, autonomous, fspec)
		})
		return r.runAdhocFlow(rctx, caller, autonomous, spec, run)
	}
	return tools.WithRunAdhocFlow(ctx, fn)
}

// runAdhocFlow executes a run_adhoc_flow plan on the orchestration engine.
//
// The submitted graph is written as a hidden (Ephemeral) Flow row and the run is
// an ordinary FlowRun attached to it, so get_view kind=flowrun, cancellation and
// run lineage work unchanged; the row never appears in the flow catalog. Before
// any row exists the plan is compiled, structurally validated (Graph.Validate +
// validateFlowPreconditions) and every leg target resolved, so a bad plan leaves
// nothing behind.
//
// run carries the fan-out binding; its maxRounds is set here from spec.
func (r *Runtime) runAdhocFlow(ctx context.Context, caller db.Agent, autonomous bool, spec tools.AdhocFlowSpec, run *adhocRun) (tools.AdhocFlowResult, error) {
	maxRounds := spec.MaxRounds
	if maxRounds == 0 {
		maxRounds = AdhocFlowMaxRounds
	}
	if maxRounds < 1 || maxRounds > AdhocFlowMaxRounds {
		return tools.AdhocFlowResult{}, fmt.Errorf("\"max_rounds\" must be between 1 and %d; got %d", AdhocFlowMaxRounds, spec.MaxRounds)
	}
	run.maxRounds = maxRounds

	g, err := compileAdhocFlow(spec)
	if err != nil {
		return tools.AdhocFlowResult{}, err
	}
	if err := g.Validate(); err != nil {
		return tools.AdhocFlowResult{}, fmt.Errorf("ad-hoc plan does not compile to a valid flow: %w", err)
	}
	if err := r.validateFlowPreconditions(ctx, g); err != nil {
		return tools.AdhocFlowResult{}, err
	}
	// Legs persist as child sessions of the calling one; without a session every
	// leg would fail at its first step, so refuse the plan up front instead.
	if SessionIDFrom(ctx) == "" {
		return tools.AdhocFlowResult{}, fmt.Errorf("run_adhoc_flow requires a calling session")
	}
	if err := r.resolveAdhocTargets(ctx, caller, spec); err != nil {
		return tools.AdhocFlowResult{}, err
	}

	graph, err := json.Marshal(g)
	if err != nil {
		return tools.AdhocFlowResult{}, fmt.Errorf("encode ad-hoc graph: %w", err)
	}
	flow, err := r.db.CreateFlow(ctx, db.Flow{
		Name:      adhocFlowName(spec),
		Graph:     string(graph),
		Ephemeral: true,
		CreatedBy: caller.ID,
	})
	if err != nil {
		return tools.AdhocFlowResult{}, fmt.Errorf("store ad-hoc flow: %w", err)
	}

	// The budget counter stays the turn's shared one; only its cap widens for the
	// rounds of this run (and anything nested beneath them).
	ctx = withDelegationCeiling(ctx, adhocDelegationCeiling(r.tun.DelegationMaxCalls(), maxRounds))
	ctx = withAdhocRun(ctx, run)

	parentRunID, parentNodeID, rootRunID := runLineage(ctx)
	fr, err := r.db.CreateFlowRun(ctx, db.FlowRun{
		FlowID:      flow.ID,
		ParentRunID: parentRunID, ParentNodeID: parentNodeID, RootRunID: rootRunID,
	})
	if err != nil {
		return tools.AdhocFlowResult{}, fmt.Errorf("create ad-hoc flow run: %w", err)
	}
	r.emitFlowRunEvent(fr)
	r.logger.Info("ad-hoc flow run started", "flow", flow.ID, "run", fr.ID, "steps", len(spec.Steps), "maxRounds", maxRounds)

	final := r.driveFlow(ctx, fr, g, "", orchestration.NewState(g), autonomous, nil)
	return buildAdhocResult(spec, final, run, ctx.Err() != nil)
}

// resolveAdhocTargets checks every leg target against the calling agent before
// the run starts — the same resolution runAgent performs per leg — so a typo in
// round two is refused up front instead of after round one already spent budget.
func (r *Runtime) resolveAdhocTargets(ctx context.Context, caller db.Agent, spec tools.AdhocFlowSpec) error {
	for _, s := range spec.Steps {
		for i, t := range s.Tasks {
			if _, _, err := r.resolveSubagentTarget(ctx, caller, t.Target); err != nil {
				return fmt.Errorf("step %q task %d: %w", s.ID, i, err)
			}
		}
	}
	return nil
}

// adhocFlowName is the hidden flow row's name — never shown in the catalog, but
// the run views title the run with it.
func adhocFlowName(spec tools.AdhocFlowSpec) string {
	ids := make([]string, len(spec.Steps))
	for i, s := range spec.Steps {
		ids[i] = s.ID
	}
	return "ad-hoc: " + strings.Join(ids, " → ")
}

// buildAdhocResult maps a finished FlowRun onto the tool result, one entry per
// plan step. Statuses are derived, not tracked: a step absent from the engine
// trace (and with no recorded fan-out) was never entered — skipped; a parallel
// step's leg outcomes come from the fan-out record, not from the trace.
func buildAdhocResult(spec tools.AdhocFlowSpec, fr db.FlowRun, run *adhocRun, cancelled bool) (tools.AdhocFlowResult, error) {
	var st orchestration.State
	if err := json.Unmarshal([]byte(fr.State), &st); err != nil {
		return tools.AdhocFlowResult{}, fmt.Errorf("ad-hoc run %s finished (%s) but its state is unreadable: %w", fr.ID, fr.Status, err)
	}
	entered := map[string]bool{}
	routed := map[string]string{} // route node id -> its last "→ label" output
	for _, t := range st.Trace {
		entered[t.NodeID] = true
		if t.Type == orchestration.NodeBranch {
			routed[t.NodeID] = t.Output
		}
	}

	res := tools.AdhocFlowResult{RunID: fr.ID, Final: fr.Output, Error: fr.Error}
	switch {
	case fr.Status == db.FlowSuccess:
		res.Status = tools.AdhocStatusDone
	case cancelled:
		res.Status = tools.AdhocStatusCancelled
	default:
		res.Status = tools.AdhocStatusFailed
	}

	for _, s := range spec.Steps {
		out := tools.AdhocFlowStepResult{ID: s.ID, Status: tools.AdhocStatusSkipped}
		switch s.Type {
		case tools.AdhocStepParallel:
			rec, ran := run.step(s.ID)
			switch {
			case !ran:
			case rec.err != nil && cancelled:
				out.Status = tools.AdhocStatusCancelled
				out.Output = rec.err.Error()
			case rec.err != nil:
				out.Status = tools.AdhocStatusFailed
				out.Output = rec.err.Error()
			default:
				out.Status = tools.AdhocStatusDone
				out.Output = adhocStepSummary(rec)
			}
		case tools.AdhocStepBranch:
			if entered[s.ID] {
				out.Status = tools.AdhocStatusDone
				out.Output = routed[adhocRouteNodeID(s.ID)]
			}
		case tools.AdhocStepEnd:
			if entered[s.ID] {
				out.Status = tools.AdhocStatusDone
			}
		}
		res.Steps = append(res.Steps, out)
	}
	return res, nil
}

// adhocStepSummary renders a parallel step's latest execution for the caller:
// every leg with its target, status and final reply (the run_subagent fan-out
// wording), prefixed with the execution count when a branch looped back to it.
func adhocStepSummary(rec adhocStepRecord) string {
	text, err := tools.FormatRunAgentResult(tools.RunAgentResult{FanOut: rec.outcomes, Strategy: tools.StrategyAll})
	if err != nil {
		return err.Error()
	}
	if rec.runs > 1 {
		return fmt.Sprintf("(ran %d times; latest run)\n%s", rec.runs, text)
	}
	return text
}

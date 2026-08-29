package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/orchestration"
)

// flowRunner adapts the Runtime to orchestration.AgentRunner. Each node runs
// through the agent's full pipeline (memory recall + tools + budget).
type flowRunner struct {
	rt         *Runtime
	autonomous bool
}

// RunAgentNode implements orchestration.AgentRunner.
func (f flowRunner) RunAgentNode(ctx context.Context, agentID, prompt string) (string, error) {
	return f.RunAgentNodeSchema(ctx, agentID, prompt, "")
}

// RunAgentNodeSchema implements orchestration.SchemaAgentRunner: an agent node
// with an OutputSchema gets its reply constrained via structured outputs on
// providers/models with support (parse-proof branch routing); elsewhere the
// schema is ignored and the reply stays free text.
func (f flowRunner) RunAgentNodeSchema(ctx context.Context, agentID, prompt, outputSchema string) (string, error) {
	agent, err := f.rt.db.GetAgent(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("agentId %q not found: %w", agentID, err)
	}
	// Volatile clock/lessons ride the dynamic suffix so the node's static system
	// prefix stays byte-stable across nodes and runs (cacheable).
	return f.rt.complete(ctx, agent, f.rt.systemPrompt(agent), f.rt.autonomousDynamicSuffix(ctx, agent), prompt, outputSchema, f.autonomous)
}

// RunAgentNodeThread implements orchestration.ThreadAgentRunner: an accumulate-mode
// agent node runs with the accumulated conversation (thread) as its prior messages,
// so the agent's stable system + growing message prefix is reused by the provider's
// prompt cache across sequential same-agent nodes.
func (f flowRunner) RunAgentNodeThread(ctx context.Context, agentID string, thread []orchestration.Msg, prompt, outputSchema string) (string, error) {
	agent, err := f.rt.db.GetAgent(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("agentId %q not found: %w", agentID, err)
	}
	return f.rt.completeThread(ctx, agent, f.rt.systemPrompt(agent), f.rt.autonomousDynamicSuffix(ctx, agent), thread, prompt, outputSchema, f.autonomous)
}

// subflowDepthKey carries the nested-subflow depth so a flow that (directly or
// transitively) calls itself can't recurse forever.
type subflowDepthKey struct{}

// maxSubflowDepth caps how deep subflow nodes may nest (a flow calling a flow
// calling a flow …). The child's own maxSteps bounds each level's work.
const maxSubflowDepth = 5

// RunChildFlow implements orchestration.ChildFlowRunner: it runs another flow to
// completion (synchronously, its own FlowRun) and returns its final output, so a
// subflow node can compose flows. Recursion is depth-guarded via the context.
func (f flowRunner) RunChildFlow(ctx context.Context, flowID, input string) (string, error) {
	depth, _ := ctx.Value(subflowDepthKey{}).(int)
	if depth >= maxSubflowDepth {
		return "", fmt.Errorf("subflow recursion too deep (>%d) — a flow is calling itself", maxSubflowDepth)
	}
	childCtx := context.WithValue(ctx, subflowDepthKey{}, depth+1)
	run, err := f.rt.RunFlow(childCtx, flowID, input, f.autonomous, nil)
	if err != nil {
		return "", err
	}
	switch run.Status {
	case db.FlowFailure:
		return "", fmt.Errorf("child flow failed: %s", run.Error)
	case db.FlowWaiting:
		// A synchronously-called subflow that suspends at await-input has no one to
		// feed it inline; surface it rather than hang the parent.
		return "", fmt.Errorf("child flow suspended at await-input (not supported inside a synchronous subflow)")
	}
	return run.Output, nil
}

// RunChildFlowResumable implements orchestration.SuspendableChildFlowRunner: like
// RunChildFlow but, instead of failing when the child suspends at await-input, it
// returns waiting=true with the child's run id so the parent subflow node can
// propagate the suspension upward and later feed input to that same child.
func (f flowRunner) RunChildFlowResumable(ctx context.Context, flowID, input string) (out, childRunID string, waiting bool, err error) {
	depth, _ := ctx.Value(subflowDepthKey{}).(int)
	if depth >= maxSubflowDepth {
		return "", "", false, fmt.Errorf("subflow recursion too deep (>%d) — a flow is calling itself", maxSubflowDepth)
	}
	childCtx := context.WithValue(ctx, subflowDepthKey{}, depth+1)
	run, err := f.rt.RunFlow(childCtx, flowID, input, f.autonomous, nil)
	if err != nil {
		return "", "", false, err
	}
	switch run.Status {
	case db.FlowFailure:
		return "", "", false, fmt.Errorf("child flow failed: %s", run.Error)
	case db.FlowWaiting:
		return "", run.ID, true, nil // propagate: parent parks on this child
	}
	return run.Output, run.ID, false, nil
}

// ResumeChildFlow implements orchestration.SuspendableChildFlowRunner: it feeds
// the parent's delivered input to a suspended child run (synchronously) and
// reports whether the child completed (out) or suspended again (waiting).
func (f flowRunner) ResumeChildFlow(ctx context.Context, childRunID, input string) (out string, waiting bool, err error) {
	run, err := f.rt.resumeWaitingFlowSync(ctx, childRunID, input)
	if err != nil {
		return "", false, err
	}
	switch run.Status {
	case db.FlowFailure:
		return "", false, fmt.Errorf("child flow failed: %s", run.Error)
	case db.FlowWaiting:
		return "", true, nil
	}
	return run.Output, false, nil
}

// joinPollInterval is how often JoinChildFlows re-checks its spawned child runs.
const joinPollInterval = 500 * time.Millisecond

// SpawnChildFlows implements orchestration.AsyncFlowRunner: it launches each flow
// as an INDEPENDENT async run (non-blocking) and returns their run ids. A join
// node later awaits them. Recursion is depth-guarded via the context (the depth
// value survives context.WithoutCancel in spawnChildFlow's goroutine).
func (f flowRunner) SpawnChildFlows(ctx context.Context, flowIDs []string, input string) ([]string, error) {
	ids := make([]string, 0, len(flowIDs))
	for _, fid := range flowIDs {
		id, err := f.rt.spawnChildFlow(ctx, fid, input, f.autonomous)
		if err != nil {
			return ids, fmt.Errorf("spawn %s: %w", fid, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// JoinChildFlows implements orchestration.AsyncFlowRunner: it block-polls the
// given child runs until every one reaches a terminal status, then returns their
// SUCCESSFUL outputs in original order. Polling runs in the parent flow's own
// goroutine.
//
// timeoutSec bounds the wait (0 = forever). partial makes the barrier tolerant:
//   - partial=false (strict): any child failure/suspension, or the timeout, fails
//     the whole join.
//   - partial=true: a failed/suspended child is dropped (excluded); at the timeout
//     the still-pending children are dropped too and the join returns what it has.
//
// A suspended (await-input) child is treated as an error case because async
// children must be non-interactive — nobody feeds them, so they'd never finish.
func (f flowRunner) JoinChildFlows(ctx context.Context, runIDs []string, timeoutSec int, partial bool, onProgress func(done, total int)) ([]string, error) {
	outputs := make([]string, len(runIDs))
	pending := map[int]string{}
	dropped := map[int]bool{}
	for i, id := range runIDs {
		pending[i] = id
	}
	total := len(runIDs)
	// resolved = finished (success + dropped); total - len(pending) once the loop
	// has processed a cycle. Report the initial state before the first poll.
	report := func() {
		if onProgress != nil {
			onProgress(total-len(pending), total)
		}
	}
	report()
	var deadline <-chan time.Time
	if timeoutSec > 0 {
		t := time.NewTimer(time.Duration(timeoutSec) * time.Second)
		defer t.Stop()
		deadline = t.C // a nil channel (timeoutSec==0) never fires → wait forever
	}
	for len(pending) > 0 {
		for i, id := range pending {
			run, err := f.rt.db.GetFlowRun(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("poll child %s: %w", id, err)
			}
			switch run.Status {
			case db.FlowSuccess:
				outputs[i] = run.Output
				delete(pending, i)
			case db.FlowFailure:
				if !partial {
					return nil, fmt.Errorf("spawned child %s failed: %s", id, run.Error)
				}
				dropped[i] = true
				delete(pending, i)
				f.rt.logger.Warn("join dropping failed child", "run", id, "error", run.Error)
			case db.FlowWaiting:
				if !partial {
					return nil, fmt.Errorf("spawned child %s suspended at await-input (async children must be non-interactive)", id)
				}
				dropped[i] = true
				delete(pending, i)
				f.rt.logger.Warn("join dropping suspended child", "run", id)
			}
		}
		report()
		if len(pending) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline:
			if !partial {
				return nil, fmt.Errorf("join timed out after %ds with %d child(ren) still pending", timeoutSec, len(pending))
			}
			for i, id := range pending {
				dropped[i] = true
				f.rt.logger.Warn("join dropping timed-out child", "run", id)
			}
			pending = map[int]string{}
		case <-time.After(joinPollInterval):
		}
	}
	// Successful outputs in original order (dropped children excluded).
	result := make([]string, 0, len(runIDs))
	for i := range runIDs {
		if !dropped[i] {
			result = append(result, outputs[i])
		}
	}
	return result, nil
}

// RunFlow starts a new run of a flow with the given input and drives it to
// completion. Manual runs (autonomous=false) are not budget-gated. obs is an
// optional progress observer (nil for no live events) used by the streaming path.
func (r *Runtime) RunFlow(ctx context.Context, flowID, input string, autonomous bool, obs orchestration.Observer) (db.FlowRun, error) {
	ctx = WithCallKind(ctx, KindFlow) // every node's provider call is attributed to orchestration
	flow, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return db.FlowRun{}, err
	}
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		return db.FlowRun{}, err
	}
	if err := g.Validate(); err != nil {
		return db.FlowRun{}, err
	}
	// Structural validity is not enough: the graph may reference an agent that was
	// deleted, or whose provider is no longer configured. Reject before a FlowRun
	// row exists, so an unrunnable flow leaves no failed run behind.
	if err := r.validateFlowPreconditions(ctx, g); err != nil {
		return db.FlowRun{}, err
	}

	// A ctx carrying a flow run id means this call came from inside another run
	// (subflow node, or an agent node's run_flow tool), so the new run joins that
	// run's tree; started standalone it is a root.
	parentRunID, parentNodeID, rootRunID := runLineage(ctx)
	run, err := r.db.CreateFlowRun(ctx, db.FlowRun{
		FlowID: flowID, Input: input,
		ParentRunID: parentRunID, ParentNodeID: parentNodeID, RootRunID: rootRunID,
	})
	if err != nil {
		return db.FlowRun{}, err
	}
	r.logger.Info("flow run started", "flow", flowID, "run", run.ID, "parent", parentRunID)
	return r.driveFlow(ctx, run, g, input, orchestration.NewState(g), autonomous, obs), nil
}

// spawnChildFlow launches a flow as an INDEPENDENT async run in its own goroutine
// and returns its run id immediately (non-blocking) — the engine's spawn node
// records the id and a later join awaits it. Preconditions are validated up front
// so a bad spawn fails fast (no orphan run). Recursion is depth-guarded via the
// context; the depth value survives context.WithoutCancel so a spawned child that
// spawns again keeps counting up toward maxSubflowDepth.
func (r *Runtime) spawnChildFlow(ctx context.Context, flowID, input string, autonomous bool) (string, error) {
	depth, _ := ctx.Value(subflowDepthKey{}).(int)
	if depth >= maxSubflowDepth {
		return "", fmt.Errorf("spawn recursion too deep (>%d) — a flow is spawning itself", maxSubflowDepth)
	}
	flow, err := r.db.GetFlow(ctx, flowID)
	if err != nil {
		return "", err
	}
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		return "", err
	}
	if err := g.Validate(); err != nil {
		return "", err
	}
	if err := r.validateFlowPreconditions(ctx, g); err != nil {
		return "", err
	}
	// Lineage is read from the SPAWNING run's ctx — driveFlow only rebinds the run
	// id to the child once it starts below, so ctx still points at the parent here.
	parentRunID, parentNodeID, rootRunID := runLineage(ctx)
	run, err := r.db.CreateFlowRun(ctx, db.FlowRun{
		FlowID: flowID, Input: input,
		ParentRunID: parentRunID, ParentNodeID: parentNodeID, RootRunID: rootRunID,
	})
	if err != nil {
		return "", err
	}
	childCtx := context.WithValue(WithCallKind(ctx, KindFlow), subflowDepthKey{}, depth+1)
	go r.driveFlow(context.WithoutCancel(childCtx), run, g, input, orchestration.NewState(g), autonomous, nil)
	return run.ID, nil
}

// waitingSweepInterval is how often the sweeper scans for timed-out await-input
// runs. Coarse on purpose — await timeouts are minutes/hours, not seconds.
const waitingSweepInterval = 30 * time.Second

// awaitTimeoutExceeded reports whether a run suspended at an await node with the
// given TimeoutSec (0 = never) has blown its deadline. updatedAt is the suspend
// time (unix seconds), now the current unix time.
func awaitTimeoutExceeded(timeoutSec int, updatedAt, now int64) bool {
	return timeoutSec > 0 && now-updatedAt >= int64(timeoutSec)
}

// StartWaitingFlowSweeper launches the background timeout sweeper: it periodically
// fails any await-input run that has out-waited its node's TimeoutSec, so a run
// nobody ever feeds can't sleep forever. Stops when ctx is cancelled.
//
// It doubles as the host for the running-counter drift check (see
// runCounterCheckEvery): that check needs a slow, always-on tick and this
// sweeper already has one, so it costs no extra goroutine or timer.
func (r *Runtime) StartWaitingFlowSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(waitingSweepInterval)
		defer t.Stop()
		ticks := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.sweepWaitingFlowsAt(ctx, time.Now().Unix())
				ticks++
				if ticks%runCounterCheckEvery == 0 {
					r.checkRunCounterDrift()
				}
			}
		}
	}()
}

// runCounterCheckEvery is how many sweeper ticks pass between running-counter
// drift checks — 20 × 30s = every 10 minutes. The check is a full store scan,
// exactly what the counter exists to avoid, so it must stay rare.
const runCounterCheckEvery = 20

// checkRunCounterDrift verifies the O(1) running-flow-run counter still agrees
// with the store, correcting it if not. Drift is a BUG — some path flipped a
// run's status without going through the counter — so it is logged at Error
// level rather than quietly repaired: the correction keeps the UI honest, but
// the defect that caused it needs to be visible.
func (r *Runtime) checkRunCounterDrift() {
	drift := r.db.ReconcileRunCounters()
	if drift.Zero() {
		return
	}
	slog.Error("running-flow-run counter drift detected; a status transition skipped the counter",
		"component", "agent", "flow_runs", drift.FlowRuns)
}

// sweepWaitingFlowsAt fails every waiting run whose await node's TimeoutSec has
// elapsed by `now`. It CAS-claims each run (waiting→running) before failing it, so
// a concurrent live resume (human/peer) always wins over the sweeper. Split from
// StartWaitingFlowSweeper with an explicit `now` so it is deterministically testable.
func (r *Runtime) sweepWaitingFlowsAt(ctx context.Context, now int64) {
	runs, err := r.db.ListWaitingFlowRuns(ctx)
	if err != nil {
		return
	}
	for _, run := range runs {
		flow, err := r.db.GetFlow(ctx, run.FlowID)
		if err != nil {
			continue
		}
		g, err := orchestration.ParseGraph(flow.Graph)
		if err != nil {
			continue
		}
		var st struct {
			WaitingAt string `json:"waitingAt"`
		}
		_ = json.Unmarshal([]byte(run.State), &st)
		node, ok := g.NodeByID(st.WaitingAt)
		if !ok || !awaitTimeoutExceeded(node.TimeoutSec, run.UpdatedAt, now) {
			continue
		}
		// Claim so we never race a live resume; if it's already been claimed the
		// human/peer won and we skip.
		if _, err := r.db.ClaimWaitingFlowRun(ctx, run.ID); err != nil {
			continue
		}
		if err := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", fmt.Sprintf("await-input timed out after %ds", node.TimeoutSec)); err != nil {
			r.logger.Warn("fail timed-out await run", "run", run.ID, "error", err)
			continue
		}
		r.logger.Info("await-input timed out", "run", run.ID, "flow", run.FlowID, "node", st.WaitingAt, "timeoutSec", node.TimeoutSec)
	}
}

// ResumeWaitingFlow delivers external input to a flow run suspended at an
// await-input node and resumes it in the background. Exactly one caller wins the
// waiting→running CAS (ClaimWaitingFlowRun), so concurrent input from multiple
// windows/peers is safe — the rest get an error. The input rides {{last}} into the
// node after the await. Resume is treated as interactive (not budget-gated).
func (r *Runtime) ResumeWaitingFlow(ctx context.Context, runID, input string) (db.FlowRun, error) {
	ctx = WithCallKind(ctx, KindFlow)
	run, g, st, err := r.prepareResume(ctx, runID, input)
	if err != nil {
		return run, err
	}
	go r.driveFlow(context.WithoutCancel(ctx), run, g, run.Input, st, false, nil)
	return run, nil
}

// resumeWaitingFlowSync is the synchronous sibling of ResumeWaitingFlow: it drives
// the resumed run inline and returns its terminal (or re-suspended) FlowRun. Used
// by subflow await-input propagation, where the parent must know the child's
// immediate outcome before deciding whether to advance or stay parked.
func (r *Runtime) resumeWaitingFlowSync(ctx context.Context, runID, input string) (db.FlowRun, error) {
	ctx = WithCallKind(ctx, KindFlow)
	run, g, st, err := r.prepareResume(ctx, runID, input)
	if err != nil {
		return run, err
	}
	return r.driveFlow(ctx, run, g, run.Input, st, false, nil), nil
}

// prepareResume CAS-claims a waiting run (waiting→running; the double-resume
// guard), loads its graph + persisted state, injects the delivered input into
// State.Last (which the await/subflow node at Current consumes), and persists it
// before any drive so a crash mid-resume can't lose the input. Shared by the
// async (ResumeWaitingFlow) and sync (resumeWaitingFlowSync) paths.
func (r *Runtime) prepareResume(ctx context.Context, runID, input string) (db.FlowRun, orchestration.Graph, orchestration.State, error) {
	run, err := r.db.ClaimWaitingFlowRun(ctx, runID)
	if err != nil {
		return db.FlowRun{}, orchestration.Graph{}, orchestration.State{}, err
	}
	flow, err := r.db.GetFlow(ctx, run.FlowID)
	if err != nil {
		// Flow deleted while waiting: fail the run cleanly.
		if finishErr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "flow deleted"); finishErr != nil {
			return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("flow %s missing and failure status persistence failed: %v (original: %w)", run.FlowID, finishErr, err)
		}
		return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("flow %s: %w", run.FlowID, err)
	}
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		if finishErr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", err.Error()); finishErr != nil {
			return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("persist graph-parse failure: %v (original: %w)", finishErr, err)
		}
		return run, orchestration.Graph{}, orchestration.State{}, err
	}
	var st orchestration.State
	if uerr := json.Unmarshal([]byte(run.State), &st); uerr != nil {
		if finishErr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "corrupted run state"); finishErr != nil {
			return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("persist corrupt-state failure: %v (original: %w)", finishErr, uerr)
		}
		return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("corrupted run state: %w", uerr)
	}
	if st.Outputs == nil {
		if finishErr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "corrupted run state: missing outputs"); finishErr != nil {
			return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("persist missing-outputs failure: %w", finishErr)
		}
		return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("corrupted run state: missing outputs")
	}
	st.Last = input
	data, err := json.Marshal(st)
	if err != nil {
		return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("marshal resumed flow state: %w", err)
	}
	if err := r.db.SetFlowRunState(ctx, run.ID, string(data)); err != nil {
		return run, orchestration.Graph{}, orchestration.State{}, fmt.Errorf("persist resumed flow state: %w", err)
	}
	return run, g, st, nil
}

// driveFlow runs the engine from the given state, persisting after each node,
// and records the terminal status. It never returns an error: a failure is
// captured in the returned FlowRun (status=failure) so callers always get a row.
// obs (optional) receives per-node progress events for live streaming.
func (r *Runtime) driveFlow(ctx context.Context, run db.FlowRun, g orchestration.Graph, input string, st orchestration.State, autonomous bool, obs orchestration.Observer) db.FlowRun {
	// A flow can run in its own goroutine (ResumeRunningFlows) or under the
	// scheduler; an unrecovered panic in a node would otherwise crash the whole
	// process. Recover it, log it, and mark the run failed so the UI/feed reflects
	// the crash instead of a run stuck forever in "running".
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error("flow run panicked", "flow", run.FlowID, "run", run.ID, "panic", p)
			if err := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", fmt.Sprintf("panic: %v", p)); err != nil {
				r.logger.Warn("finish panicked flow run failed", "run", run.ID, "error", err)
			}
		}
	}()
	// Thread the run id so each agent node's tool/thinking steps land in a
	// sidecar keyed by (runID, nodeID) — see captureFlowNodeSteps.
	ctx = withFlowRunID(ctx, run.ID)
	// Carry the tree root too, so a run started from within this one records its
	// RootRunID without reading this row back. RootOf resolves "empty = self".
	ctx = withFlowRootID(ctx, run.RootOf())
	eng := orchestration.NewEngine(flowRunner{rt: r, autonomous: autonomous})
	// Always broadcast per-node lifecycle on the process-wide bus (keyed by run
	// id) so EVERY window's run viewer renders progress live — not just the HTTP
	// client that started the run, and including autonomous/scheduled runs where
	// the caller's obs is nil. The run-starter's own per-request SSE (obs) is
	// still invoked when present. The observer may fire concurrently (parallel
	// children); r.publish is mutex-guarded so the fan-out is safe.
	eng.SetObserver(func(ev orchestration.NodeEvent) {
		r.emitFlowNode(run, ev)
		if obs != nil {
			obs(ev)
		}
	})

	checkpointID, sequence, journalErr := r.db.FlowRunStateJournalInfo(ctx, run.ID)
	if journalErr != nil {
		if err := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", fmt.Sprintf("load flow state journal: %v", journalErr)); err != nil {
			r.logger.Warn("finish corrupt-journal flow run failed", "run", run.ID, "error", err)
		}
		run.Status = db.FlowFailure
		run.Error = fmt.Sprintf("load flow state journal: %v", journalErr)
		return run
	}
	deltaWriter := newFlowStateDeltaWriter(checkpointID, sequence, st)
	save := func(s orchestration.State) error {
		delta, err := deltaWriter.next(s)
		if err != nil {
			return err
		}
		return r.db.AppendFlowRunStateDelta(ctx, run.ID, delta)
	}

	final, runErr := eng.Run(ctx, g, input, st, save)

	data, marshalErr := json.Marshal(final)
	if marshalErr != nil && runErr == nil {
		runErr = fmt.Errorf("marshal final flow state: %w", marshalErr)
	}
	if marshalErr == nil {
		run.State = string(data)
	}

	// Durable suspend: the run paused at an await-input node. Persist state +
	// waiting status and return — NOT terminal. ResumeWaitingFlow revives it when
	// input arrives; boot never auto-resumes it (it's not "running").
	if runErr == nil && final.WaitingAt != "" {
		if err := r.db.MarkFlowRunWaiting(ctx, run.ID, string(data)); err != nil {
			runErr = fmt.Errorf("checkpoint waiting flow state: %w", err)
		} else {
			run.Status = db.FlowWaiting
			run.State = string(data)
			r.logger.Info("flow run waiting for input", "flow", run.FlowID, "run", run.ID, "node", final.WaitingAt)
			return run
		}
	}
	if marshalErr == nil {
		if err := r.db.SetFlowRunState(ctx, run.ID, string(data)); err != nil && runErr == nil {
			runErr = fmt.Errorf("checkpoint final flow state: %w", err)
		}
	}

	status := db.FlowSuccess
	errText := ""
	if runErr != nil {
		status = db.FlowFailure
		errText = runErr.Error()
	}
	if err := r.db.FinishFlowRun(ctx, run.ID, status, final.Last, errText); err != nil {
		r.logger.Warn("finish flow run failed", "run", run.ID, "error", err)
	}
	run.Status = status
	run.Output = final.Last
	run.Error = errText
	r.logger.Info("flow run finished", "flow", run.FlowID, "run", run.ID, "status", status, "steps", final.Steps)
	return run
}

// RunFlowRecorded runs a flow and records the result as a turn in the flow's
// dedicated transcript session (Session.Kind "flow"), so a standalone flow run —
// like a task run — is viewable in the unified executions feed and the same
// streamable transcript as a chat. Returns the flow run, the session id, and any
// setup error. obs (optional) receives per-node progress for live streaming.
func (r *Runtime) RunFlowRecorded(ctx context.Context, flowID, input string, autonomous bool, obs orchestration.Observer) (db.FlowRun, string, error) {
	flow, ferr := r.db.GetFlow(ctx, flowID)
	if ferr != nil {
		return db.FlowRun{}, "", ferr
	}
	// Resolve the flow's transcript session up front and mark it running for the
	// duration of the execution, so the live executions feed and the network
	// graph show the flow (and its agent) as active *while* it runs — not only
	// after it finishes. The session is idempotent (one per flow), so
	// recordFlowSessionTurn below reuses the same one.
	// Each run gets its OWN session (per-run isolation): keyed for attribution by
	// SourceID = flow.ID (the network graph + executions feed still resolve it to
	// the flow) but NEVER reused, so a run's transcript — and its "Akış olarak gör"
	// reification — shows exactly one run. Created up front so the executions feed
	// shows it running, then the same session id is threaded into the turn record.
	sessionID := ""
	inputRecorded := false
	if sess, serr := r.db.CreateSession(ctx, db.Session{
		AgentID:  firstFlowAgentID(flow),
		Kind:     "flow",
		SourceID: flow.ID,
		Title:    flow.Name,
		// Inherit the launching turn's directory when there is one (run_flow from a
		// session pinned to a repo); with no caller session this resolves to the
		// workspace default, which is what a scheduled run wants.
		WorkingDir: r.effectiveWorkDir(ctx),
	}); serr == nil {
		sessionID = sess.ID
		// Own cancelable context for the whole recorded run so a human "Durdur"
		// (CancelSession) can stop the flow — it never enters the api chatRuns.
		runCtx, cancelRun := context.WithCancel(ctx)
		defer cancelRun()
		ctx = runCtx
		r.trackSession(sessionID, cancelRun)
		defer r.untrackSession(sessionID)
		// Record the user turn AND announce the session up front, so the chat
		// sidebar shows the run — with its input bubble — the instant it starts,
		// not only after it finishes. The assistant reply is appended at the end
		// (recordFlowSessionTurn, inputRecorded=true so the input isn't dup'd).
		r.recordFlowInput(ctx, flow, input, sessionID)
		inputRecorded = true
		r.publish(events.Event{
			Type:   events.TypeSession,
			Level:  "info",
			Target: map[string]string{"sessionId": sessionID, "op": "create"},
		})
	}
	run, runErr := r.RunFlow(ctx, flowID, input, autonomous, obs)
	if recorded := r.recordFlowSessionTurn(ctx, flow, run, input, runErr, sessionID, inputRecorded); recorded != "" {
		sessionID = recorded
		// Link the run to its transcript session so the chat "Akış olarak gör"
		// can resolve back to this exact run's REAL graph instead of reifying.
		if err := r.db.SetFlowRunSession(ctx, run.ID, sessionID); err != nil {
			r.logger.Warn("link flow run to session failed", "run", run.ID, "session", sessionID, "error", err)
		} else {
			run.SessionID = sessionID
		}
		// The assistant reply just landed — nudge any window viewing this session
		// to reload its transcript (op "message_added" triggers listMessages).
		r.publish(events.Event{
			Type:   events.TypeSession,
			Level:  "info",
			Target: map[string]string{"sessionId": sessionID, "op": "message_added"},
		})
	}
	// Every recorded flow run raises a desktop notification that deep-links to the
	// run's transcript in the executions feed. This covers both autonomous
	// (background) runs and interactive runs the user started then switched away
	// from: the client-side notify() only shows a toast when the window is
	// backgrounded, so a user actively watching the live stream is never spammed.
	if sessionID != "" {
		r.emitFlowDelivery(flow, run, sessionID, runErr)
	}
	return run, sessionID, runErr
}

// emitFlowNode broadcasts one flow node lifecycle event on the process-wide bus,
// tagged with the run id, so any window viewing that run (the Koşular tab's
// RunView) renders node start/done/error + output live instead of waiting for
// the ~3s state poll. Best-effort: run state is persisted after every node, so a
// dropped frame only costs a little latency, never correctness.
//
// The frame also carries the run's LINEAGE, taken straight off the run row (no
// extra read):
//
//   - rootRunId — the TOP of the run's tree, so a viewer watching a composed run
//     hears its subflow/spawn children too, including children that do not exist
//     yet when it subscribes (each child broadcasts under its own run id, which
//     the viewer cannot know in advance). Equals flowRunId for a root run.
//   - parentRunId + parentNodeId — WHERE this run hangs in the tree: the run that
//     launched it and the node in that run's graph which did. A parent's canvas
//     rolls a child's live progress up onto that node; without the tags it would
//     have to fetch the run tree to place a just-born child, and would show
//     nothing until that fetch returned. Both are needed, not just the node: node
//     ids are only unique WITHIN one flow graph, so two runs in the same tree can
//     each own an "n1" and a node-only key would paint one onto the other's
//     canvas. Empty for a root run, and parentNodeId is also empty for a child
//     started by the run_flow tool from outside any node.
func (r *Runtime) emitFlowNode(run db.FlowRun, ev orchestration.NodeEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:  "flow_node",
		Level: "info",
		Target: map[string]string{
			"flowRunId":    run.ID,
			"flowId":       run.FlowID,
			"rootRunId":    run.RootOf(),
			"parentRunId":  run.ParentRunID,
			"parentNodeId": run.ParentNodeID,
		},
		Node: b,
	})
}

// emitFlowDelivery publishes the outcome of an autonomous flow run as a desktop
// notification that deep-links to the run's transcript in the executions feed
// (Session.Kind "flow"), so clicking opens the per-node transcript inline.
func (r *Runtime) emitFlowDelivery(flow db.Flow, run db.FlowRun, sessionID string, runErr error) {
	target := map[string]string{"view": "executions", "sessionId": sessionID}
	if runErr != nil || run.Status == db.FlowFailure {
		body := ""
		if runErr != nil {
			body = notifyLine(runErr.Error(), 200)
		} else if run.Error != "" {
			body = notifyLine(run.Error, 200)
		}
		r.publish(events.Event{
			Type:   events.TypeFlow,
			Level:  "error",
			Title:  "🔀 Akış başarısız — " + flow.Name,
			Body:   body,
			Target: target,
		})
		return
	}
	r.publish(events.Event{
		Type:   events.TypeFlow,
		Level:  "success",
		Title:  "🔀 Akış tamamlandı — " + flow.Name,
		Body:   notifyLine(run.Output, 120),
		Target: target,
	})
}

// flowInputText is the user-turn text for a recorded flow run: the trimmed
// input, or a "🔀 <flow name>" placeholder when the flow takes no input.
func flowInputText(flow db.Flow, input string) string {
	if t := strings.TrimSpace(input); t != "" {
		return t
	}
	return "🔀 " + flow.Name
}

// recordFlowInput appends the user turn to a flow's transcript session up front
// (before the run executes), so the session is non-empty the moment it appears
// in the sidebar. Best-effort; a failure only means the input bubble is missing.
func (r *Runtime) recordFlowInput(ctx context.Context, flow db.Flow, input, sessionID string) {
	if sessionID == "" {
		return
	}
	if _, err := r.recordInjectedUserNote(ctx, sessionID, "", flowInputText(flow, input)); err != nil {
		r.logger.Warn("flow transcript: record input failed", "flow", flow.ID, "session", sessionID, "error", err)
	}
}

// recordFlowSessionTurn appends the input (user turn) and the run transcript
// (assistant turn with a per-node step trace) to the flow's transcript session,
// returning the session id. The session is grouped under the flow's first agent;
// the reply is attributed to the agent that produced the final output.
// inputRecorded=true means the user turn was already written up front (by
// recordFlowInput), so only the assistant reply is appended here.
func (r *Runtime) recordFlowSessionTurn(ctx context.Context, flow db.Flow, run db.FlowRun, input string, runErr error, sessionID string, inputRecorded bool) string {
	owner := firstFlowAgentID(flow)
	if sessionID == "" {
		// The up-front create failed (or a caller passed none) — make the per-run
		// session now so the run is still recorded somewhere.
		sess, err := r.db.CreateSession(ctx, db.Session{AgentID: owner, Kind: "flow", SourceID: flow.ID, Title: flow.Name, WorkingDir: r.effectiveWorkDir(ctx)})
		if err != nil {
			r.logger.Warn("flow session create failed", "flow", flow.ID, "error", err)
			return ""
		}
		sessionID = sess.ID
	}
	if !inputRecorded {
		if _, err := r.recordInjectedUserNote(ctx, sessionID, "", flowInputText(flow, input)); err != nil {
			r.logger.Warn("flow transcript: record input failed", "flow", flow.ID, "session", sessionID, "error", err)
		}
	}

	replyAgent := finalFlowAgentID(flow, run)
	if replyAgent == "" {
		replyAgent = owner
	}
	text := run.Output
	if text == "" {
		text = renderFlowTranscript(flow.Name, run, runErr)
	}
	if _, err := r.db.AddMessage(ctx, db.Message{
		SessionID: sessionID,
		AgentID:   replyAgent,
		Role:      "assistant",
		Text:      text,
		Steps:     encodeSteps(flowStateToSteps(run, runErr)),
	}); err != nil {
		r.logger.Warn("flow transcript: record reply failed", "flow", flow.ID, "session", sessionID, "error", err)
	}
	return sessionID
}

// flowStateToSteps converts a finished flow run's persisted state into a turn
// trace: one text step per executed node (title + output), plus an error step on
// setup/run failure. Lets the chat renderer show a flow run's per-node breakdown
// exactly like a normal turn's activity trace.
func flowStateToSteps(fr db.FlowRun, setupErr error) []TurnStep {
	if setupErr != nil {
		return []TurnStep{{Kind: StepError, Reason: "flow_setup", Text: setupErr.Error()}}
	}
	var st orchestration.State
	if json.Unmarshal([]byte(fr.State), &st) != nil {
		return nil
	}
	steps := make([]TurnStep, 0, len(st.Trace)+1)
	for _, t := range st.Trace {
		title := t.Title
		if title == "" {
			title = t.NodeID
		}
		steps = append(steps, TurnStep{Kind: StepText, Text: "**" + title + "**\n\n" + t.Output})
	}
	if fr.Status == db.FlowFailure {
		steps = append(steps, TurnStep{Kind: StepError, Reason: "flow_failure", Text: fr.Error})
	}
	return steps
}

// firstFlowAgentID returns the agentId of the graph's start node (or first agent
// node found), used as the flow session's representative owner. Empty if none.
func firstFlowAgentID(flow db.Flow) string {
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		return ""
	}
	if n, ok := g.NodeByID(g.Start); ok && nodeRunsAgent(n) {
		return n.AgentID
	}
	for _, n := range g.Nodes {
		if nodeRunsAgent(n) {
			return n.AgentID
		}
	}
	return ""
}

// nodeRunsAgent reports whether a node executes an assigned agent, so the flow
// session's owner/reply attribution covers coordinator nodes as well as plain
// agent ones.
func nodeRunsAgent(n orchestration.Node) bool {
	return (n.Type == orchestration.NodeAgent || n.Type == orchestration.NodeCoordinator) && n.AgentID != ""
}

// finalFlowAgentID returns the agent of the last agent node that executed in the
// run, so the resulting reply is attributed to whoever produced the final output.
func finalFlowAgentID(flow db.Flow, run db.FlowRun) string {
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		return ""
	}
	var st orchestration.State
	if json.Unmarshal([]byte(run.State), &st) != nil {
		return ""
	}
	for i := len(st.Trace) - 1; i >= 0; i-- {
		if n, ok := g.NodeByID(st.Trace[i].NodeID); ok && nodeRunsAgent(n) {
			return n.AgentID
		}
	}
	return ""
}

// ResumeRunningFlows continues any flow runs left in the running state (e.g.
// after a crash/restart) from their persisted state — the restart-safe path.
func (r *Runtime) ResumeRunningFlows(ctx context.Context) {
	runs, err := r.db.ListRunningFlowRuns(ctx)
	if err != nil {
		r.logger.Warn("list running flow runs failed", "error", err)
		return
	}
	for _, run := range runs {
		flow, err := r.db.GetFlow(ctx, run.FlowID)
		if err != nil {
			r.logger.Warn("resume: flow missing", "run", run.ID, "error", err)
			if ferr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "flow deleted"); ferr != nil {
				r.logger.Warn("resume: finish missing-flow run failed", "run", run.ID, "error", ferr)
			}
			continue
		}
		g, err := orchestration.ParseGraph(flow.Graph)
		if err != nil {
			r.logger.Warn("resume: flow graph parse failed", "run", run.ID, "flow", run.FlowID, "error", err)
			if ferr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", err.Error()); ferr != nil {
				r.logger.Warn("resume: finish failed run failed", "run", run.ID, "error", ferr)
			}
			continue
		}
		var st orchestration.State
		if err := json.Unmarshal([]byte(run.State), &st); err != nil || st.Outputs == nil {
			restoreErr := err
			if restoreErr == nil {
				restoreErr = fmt.Errorf("missing outputs")
			}
			r.logger.Warn("resume: flow state restore failed", "run", run.ID, "error", restoreErr)
			if ferr := r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", fmt.Sprintf("corrupted run state: %v", restoreErr)); ferr != nil {
				r.logger.Warn("resume: finish corrupt-state run failed", "run", run.ID, "error", ferr)
			}
			continue
		}
		r.logger.Info("resuming flow run", "run", run.ID, "from", st.Current)
		go r.driveFlow(ctx, run, g, run.Input, st, true, nil)
	}
}

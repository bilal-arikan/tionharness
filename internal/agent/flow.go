package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
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
		return "", err
	}
	// Volatile clock/goal ride the dynamic suffix so the node's static system
	// prefix stays byte-stable across nodes and runs (cacheable).
	return f.rt.complete(ctx, agent, f.rt.systemPrompt(agent), f.rt.autonomousDynamicSuffix(ctx), prompt, outputSchema, f.autonomous)
}

// RunAgentNodeThread implements orchestration.ThreadAgentRunner: an accumulate-mode
// agent node runs with the accumulated conversation (thread) as its prior messages,
// so the agent's stable system + growing message prefix is reused by the provider's
// prompt cache across sequential same-agent nodes.
func (f flowRunner) RunAgentNodeThread(ctx context.Context, agentID string, thread []orchestration.Msg, prompt, outputSchema string) (string, error) {
	agent, err := f.rt.db.GetAgent(ctx, agentID)
	if err != nil {
		return "", err
	}
	return f.rt.completeThread(ctx, agent, f.rt.systemPrompt(agent), f.rt.autonomousDynamicSuffix(ctx), thread, prompt, outputSchema, f.autonomous)
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

	run, err := r.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flowID, Input: input})
	if err != nil {
		return db.FlowRun{}, err
	}
	r.logger.Info("flow run started", "flow", flowID, "run", run.ID)
	return r.driveFlow(ctx, run, g, input, orchestration.NewState(g), autonomous, obs), nil
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
func (r *Runtime) StartWaitingFlowSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(waitingSweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				r.sweepWaitingFlowsAt(ctx, time.Now().Unix())
			}
		}
	}()
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
	run, err := r.db.ClaimWaitingFlowRun(ctx, runID) // CAS waiting→running (double-resume guard)
	if err != nil {
		return db.FlowRun{}, err
	}
	flow, err := r.db.GetFlow(ctx, run.FlowID)
	if err != nil {
		// Flow deleted while waiting: fail the run cleanly.
		_ = r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "flow deleted")
		return run, fmt.Errorf("flow %s: %w", run.FlowID, err)
	}
	g, err := orchestration.ParseGraph(flow.Graph)
	if err != nil {
		_ = r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", err.Error())
		return run, err
	}
	var st orchestration.State
	if uerr := json.Unmarshal([]byte(run.State), &st); uerr != nil || st.Outputs == nil {
		_ = r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "corrupted run state")
		return run, fmt.Errorf("corrupted run state: %w", uerr)
	}
	// Inject the delivered input; the await node (Current == WaitingAt) consumes it.
	st.Last = input
	// Persist the injected input BEFORE launching so a crash during resume can't lose
	// it (boot would then resume a running run whose state already carries the input).
	if data, merr := json.Marshal(st); merr == nil {
		_ = r.db.SetFlowRunState(ctx, run.ID, string(data))
	}
	go r.driveFlow(context.WithoutCancel(ctx), run, g, run.Input, st, false, nil)
	return run, nil
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
	eng := orchestration.NewEngine(flowRunner{rt: r, autonomous: autonomous})
	// Always broadcast per-node lifecycle on the process-wide bus (keyed by run
	// id) so EVERY window's run viewer renders progress live — not just the HTTP
	// client that started the run, and including autonomous/scheduled runs where
	// the caller's obs is nil. The run-starter's own per-request SSE (obs) is
	// still invoked when present. The observer may fire concurrently (parallel
	// children); r.publish is mutex-guarded so the fan-out is safe.
	eng.SetObserver(func(ev orchestration.NodeEvent) {
		r.emitFlowNode(run.ID, run.FlowID, ev)
		if obs != nil {
			obs(ev)
		}
	})

	save := func(s orchestration.State) error {
		data, err := json.Marshal(s)
		if err != nil {
			return err
		}
		return r.db.SetFlowRunState(ctx, run.ID, string(data))
	}

	final, runErr := eng.Run(ctx, g, input, st, save)

	// Best-effort final state snapshot (in case the last save raced the error).
	if data, err := json.Marshal(final); err == nil {
		_ = r.db.SetFlowRunState(ctx, run.ID, string(data))
		run.State = string(data)
	}

	// Durable suspend: the run paused at an await-input node. Persist state +
	// waiting status and return — NOT terminal. ResumeWaitingFlow revives it when
	// input arrives; boot never auto-resumes it (it's not "running").
	if runErr == nil && final.WaitingAt != "" {
		data, _ := json.Marshal(final)
		if err := r.db.MarkFlowRunWaiting(ctx, run.ID, string(data)); err != nil {
			r.logger.Warn("mark flow run waiting failed", "run", run.ID, "error", err)
		}
		run.Status = db.FlowWaiting
		run.State = string(data)
		r.logger.Info("flow run waiting for input", "flow", run.FlowID, "run", run.ID, "node", final.WaitingAt)
		return run
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
	if sess, serr := r.db.CreateSession(ctx, db.Session{
		AgentID:  firstFlowAgentID(flow),
		Kind:     "flow",
		SourceID: flow.ID,
		Title:    flow.Name,
	}); serr == nil {
		sessionID = sess.ID
		r.trackSession(sessionID)
		defer r.untrackSession(sessionID)
	}
	run, runErr := r.RunFlow(ctx, flowID, input, autonomous, obs)
	if recorded := r.recordFlowSessionTurn(ctx, flow, run, input, runErr, sessionID); recorded != "" {
		sessionID = recorded
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
func (r *Runtime) emitFlowNode(runID, flowID string, ev orchestration.NodeEvent) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   "flow_node",
		Level:  "info",
		Target: map[string]string{"flowRunId": runID, "flowId": flowID},
		Node:   b,
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

// recordFlowSessionTurn appends the input (user turn) and the run transcript
// (assistant turn with a per-node step trace) to the flow's transcript session,
// returning the session id. The session is grouped under the flow's first agent;
// the reply is attributed to the agent that produced the final output.
func (r *Runtime) recordFlowSessionTurn(ctx context.Context, flow db.Flow, run db.FlowRun, input string, runErr error, sessionID string) string {
	owner := firstFlowAgentID(flow)
	if sessionID == "" {
		// The up-front create failed (or a caller passed none) — make the per-run
		// session now so the run is still recorded somewhere.
		sess, err := r.db.CreateSession(ctx, db.Session{AgentID: owner, Kind: "flow", SourceID: flow.ID, Title: flow.Name})
		if err != nil {
			r.logger.Warn("flow session create failed", "flow", flow.ID, "error", err)
			return ""
		}
		sessionID = sess.ID
	}
	userText := strings.TrimSpace(input)
	if userText == "" {
		userText = "🔀 " + flow.Name
	}
	if _, err := r.db.AddMessage(ctx, db.Message{SessionID: sessionID, Role: "user", Text: userText}); err != nil {
		r.logger.Warn("flow transcript: record input failed", "flow", flow.ID, "session", sessionID, "error", err)
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
	if n, ok := g.NodeByID(g.Start); ok && n.Type == orchestration.NodeAgent && n.AgentID != "" {
		return n.AgentID
	}
	for _, n := range g.Nodes {
		if n.Type == orchestration.NodeAgent && n.AgentID != "" {
			return n.AgentID
		}
	}
	return ""
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
		if n, ok := g.NodeByID(st.Trace[i].NodeID); ok && n.Type == orchestration.NodeAgent {
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
			if err != nil {
				r.logger.Warn("resume: flow state restore failed, restarting from scratch", "run", run.ID, "error", err)
			}
			st = orchestration.NewState(g)
		}
		r.logger.Info("resuming flow run", "run", run.ID, "from", st.Current)
		go r.driveFlow(ctx, run, g, run.Input, st, true, nil)
	}
}

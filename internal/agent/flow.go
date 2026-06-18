package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/orchestration"
)

// flowRunner adapts the Runtime to orchestration.AgentRunner. Each node runs
// through the agent's full pipeline (memory recall + tools + budget).
type flowRunner struct {
	rt         *Runtime
	autonomous bool
}

// RunAgentNode implements orchestration.AgentRunner.
func (f flowRunner) RunAgentNode(ctx context.Context, agentID, prompt string) (string, error) {
	agent, err := f.rt.db.GetAgent(ctx, agentID)
	if err != nil {
		return "", err
	}
	dynamic := strings.TrimSpace(f.rt.mem.ContextBlock(ctx, agentID, prompt, 5))
	return f.rt.complete(ctx, agent, f.rt.systemPrompt(agent), dynamic, prompt, f.autonomous)
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

	run, err := r.db.CreateFlowRun(ctx, db.FlowRun{FlowID: flowID, Input: input})
	if err != nil {
		return db.FlowRun{}, err
	}
	r.logger.Info("flow run started", "flow", flowID, "run", run.ID)
	return r.driveFlow(ctx, run, g, input, orchestration.NewState(g), autonomous, obs), nil
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
	eng := orchestration.NewEngine(flowRunner{rt: r, autonomous: autonomous})
	if obs != nil {
		eng.SetObserver(obs)
	}

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
	run, runErr := r.RunFlow(ctx, flowID, input, autonomous, obs)
	sessionID := r.recordFlowSessionTurn(ctx, flow, run, input, runErr)
	return run, sessionID, runErr
}

// recordFlowSessionTurn appends the input (user turn) and the run transcript
// (assistant turn with a per-node step trace) to the flow's transcript session,
// returning the session id. The session is grouped under the flow's first agent;
// the reply is attributed to the agent that produced the final output.
func (r *Runtime) recordFlowSessionTurn(ctx context.Context, flow db.Flow, run db.FlowRun, input string, runErr error) string {
	owner := firstFlowAgentID(flow)
	session, err := r.db.GetOrCreateSourceSession(ctx, "flow", flow.ID, owner, flow.Name)
	if err != nil {
		r.logger.Warn("flow session create failed", "flow", flow.ID, "error", err)
		return ""
	}
	userText := strings.TrimSpace(input)
	if userText == "" {
		userText = "🔀 " + flow.Name
	}
	if _, err := r.db.AddMessage(ctx, db.Message{SessionID: session.ID, Role: "user", Text: userText}); err != nil {
		r.logger.Warn("flow transcript: record input failed", "flow", flow.ID, "session", session.ID, "error", err)
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
		SessionID: session.ID,
		AgentID:   replyAgent,
		Role:      "assistant",
		Text:      text,
		Steps:     encodeSteps(flowStateToSteps(run, runErr)),
	}); err != nil {
		r.logger.Warn("flow transcript: record reply failed", "flow", flow.ID, "session", session.ID, "error", err)
	}
	return session.ID
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

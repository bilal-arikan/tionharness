package agent

import (
	"context"
	"encoding/json"
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
	return f.rt.complete(ctx, agent, buildSystemPrompt(agent), dynamic, prompt, f.autonomous)
}

// RunFlow starts a new run of a flow with the given input and drives it to
// completion. Manual runs (autonomous=false) are not budget-gated.
func (r *Runtime) RunFlow(ctx context.Context, flowID, input string, autonomous bool) (db.FlowRun, error) {
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
	return r.driveFlow(ctx, run, g, input, orchestration.NewState(g), autonomous), nil
}

// driveFlow runs the engine from the given state, persisting after each node,
// and records the terminal status. It never returns an error: a failure is
// captured in the returned FlowRun (status=failure) so callers always get a row.
func (r *Runtime) driveFlow(ctx context.Context, run db.FlowRun, g orchestration.Graph, input string, st orchestration.State, autonomous bool) db.FlowRun {
	eng := orchestration.NewEngine(flowRunner{rt: r, autonomous: autonomous})

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
			_ = r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", "flow deleted")
			continue
		}
		g, err := orchestration.ParseGraph(flow.Graph)
		if err != nil {
			_ = r.db.FinishFlowRun(ctx, run.ID, db.FlowFailure, "", err.Error())
			continue
		}
		var st orchestration.State
		if err := json.Unmarshal([]byte(run.State), &st); err != nil || st.Outputs == nil {
			st = orchestration.NewState(g)
		}
		r.logger.Info("resuming flow run", "run", run.ID, "from", st.Current)
		go r.driveFlow(ctx, run, g, run.Input, st, true)
	}
}

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/flow"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Evolving flows (_Docs/93). Every turn of a user-facing agent is one run of
// the agent's main flow: the turn's input enters at the input node, each llm
// node is one model call made with the turn's own request (history, tools,
// permissions), and the output node's text becomes the reply. The default flow
// (input → respond → output) is behaviourally the plain turn, so an agent that
// never evolves pays nothing but one run row per turn.

// flowRunIDKey carries the active run id to the node completions (judge ledger
// refs, nested guards).
type flowRunIDKey struct{}

func withFlowRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, flowRunIDKey{}, id)
}

func flowRunIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(flowRunIDKey{}).(string)
	return id
}

// flowNodeOutputExcerpt bounds the output text carried on a node step card.
const flowNodeOutputExcerpt = 600

// flowForTurn decides whether a completion runs through the agent's flow and
// loads it. System agents, auxiliary calls (title/summary/compaction/…) and
// calls already inside a node take the direct path. A flow that fails to parse
// is reported and skipped rather than failing the turn.
func (r *Runtime) flowForTurn(ctx context.Context, agent db.Agent) (db.Flow, flow.Graph, bool) {
	if r == nil || r.db == nil || agent.ID == "" || agent.System {
		return db.Flow{}, flow.Graph{}, false
	}
	if k := callKindFrom(ctx); isAuxiliaryKind(k) || k == KindDecide {
		return db.Flow{}, flow.Graph{}, false
	}
	if flowRunIDFrom(ctx) != "" {
		return db.Flow{}, flow.Graph{}, false
	}
	f, err := r.db.EnsureAgentFlow(ctx, agent.ID)
	if err != nil {
		return db.Flow{}, flow.Graph{}, false
	}
	g, err := flow.Parse(f.Graph)
	if err != nil {
		r.logger.Warn("flow: head graph unreadable, running the plain turn", "flow", f.ID, "agent", agent.ID, "error", err)
		return db.Flow{}, flow.Graph{}, false
	}
	return f, g, true
}

// turnFlowRunner adapts one agent turn to flow.Runner: every llm node is an
// executeTurn call built from the turn's base request.
type turnFlowRunner struct {
	r          *Runtime
	run        db.FlowRun
	base       providers.Request
	input      string // the turn's input text (flowInputText), for the plain-turn equivalence
	agent      db.Agent
	provider   providers.Provider
	autonomous bool
	trivial    bool
	onStep     func(TurnStep)

	steps         []TurnStep
	usage         providers.Usage
	providerCalls int
	lastModel     string
	lastStop      string
	// threadSessionID is the CLI resume id of the last thread-mode node: the
	// one the chat path must keep, since a fresh node resumes nothing.
	threadSessionID string
	lastResp        *providers.Response
}

// RunLLM implements flow.Runner.
func (t *turnFlowRunner) RunLLM(ctx context.Context, node flow.Node, prompt string, visit int) (string, error) {
	agent, provider := t.agent, t.provider
	if node.AgentID != "" && node.AgentID != t.agent.ID {
		other, err := t.r.db.GetAgent(ctx, node.AgentID)
		if err != nil {
			return "", fmt.Errorf("node agent %q: %w", node.AgentID, err)
		}
		if err := other.RunnableErr(); err != nil {
			return "", fmt.Errorf("node agent %q: %w", node.AgentID, err)
		}
		p, err := t.r.providers.Get(other.ProviderRef())
		if err != nil {
			return "", fmt.Errorf("node agent %q provider: %w", node.AgentID, err)
		}
		agent, provider = other, p
	}
	if m := strings.TrimSpace(node.Model); m != "" {
		agent.Model = m
	}
	if node.Tools == flow.ToolsNone {
		agent.MCPEnabled = false
	}
	req := t.base
	req.Messages = append([]providers.Message(nil), t.base.Messages...)
	fresh := node.Context == flow.ContextFresh || agent.ID != t.agent.ID
	if fresh {
		// Only the agent's static prefix and the rendered prompt: no history, no
		// running summary, no CLI conversation to resume.
		req.Messages = []providers.Message{{Role: providers.RoleUser, Text: prompt}}
		req.Summary = ""
		req.ResumeSessionID = ""
		if agent.ID != t.agent.ID {
			req.System = t.r.systemPrompt(agent)
			req.SystemDynamic = t.r.autonomousDynamicSuffix(ctx, agent)
		}
	} else {
		n := len(req.Messages)
		switch {
		case n > 0 && req.Messages[n-1].Role == providers.RoleUser:
			last := req.Messages[n-1]
			if last.Text != prompt {
				last.Text = prompt
				last.RawContent = nil
			}
			req.Messages[n-1] = last
		case prompt != t.input:
			// A real stage prompt after a non-user tail (a peer agent's reply
			// in a multi-agent session) becomes the new user turn.
			req.Messages = append(req.Messages, providers.Message{Role: providers.RoleUser, Text: prompt})
		default:
			// Plain {{input}} stage: the request goes out exactly as the chat
			// path built it, assistant tail and all (the plain-turn equivalence).
		}
	}
	if s := strings.TrimSpace(node.OutputSchema); s != "" {
		req.OutputSchema = json.RawMessage(s)
	} else if node.ID != "" {
		req.OutputSchema = t.base.OutputSchema
	}
	resp, steps, err := t.r.executeTurn(ctx, agent, provider, req, t.autonomous, t.nodeStepSink())
	t.steps = append(t.steps, steps...)
	if resp != nil {
		t.usage = sumUsage(t.usage, resp.Usage)
		t.providerCalls += max(resp.ProviderCalls, 1)
		if resp.Model != "" {
			t.lastModel = resp.Model
		}
		t.lastStop = resp.StopReason
		if !fresh && resp.SessionID != "" {
			t.threadSessionID = resp.SessionID
		}
		t.lastResp = resp
	}
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("provider returned no response")
	}
	return resp.Text, nil
}

// nodeStepSink forwards a node's live steps. Inside a non-trivial flow the
// transient text deltas are dropped: several nodes streaming into one live
// bubble would read as one garbled answer, and the final text arrives with the
// reply anyway. Everything else (tools, thinking, narration) streams as usual.
func (t *turnFlowRunner) nodeStepSink() func(TurnStep) {
	if t.onStep == nil {
		return nil
	}
	if t.trivial {
		return t.onStep
	}
	return func(st TurnStep) {
		if st.Kind == StepDelta {
			return
		}
		t.onStep(st)
	}
}

// Judge implements flow.Runner (route nodes in judge mode).
func (t *turnFlowRunner) Judge(ctx context.Context, node flow.Node, value string, options []string) (int, error) {
	return t.r.judgeRouteArm(ctx, t.agent, node, value, options)
}

// Check implements flow.Runner (criteria routes).
func (t *turnFlowRunner) Check(ctx context.Context, node flow.Node, value string, criteria []string) ([]bool, error) {
	return t.r.checkRouteCriteria(ctx, t.agent, node, value, criteria)
}

// runFlowTurn drives one turn through the agent's flow and synthesizes the
// provider response the caller expects: the output node's text, the summed
// usage, the last model and the thread's resume id.
func (r *Runtime) runFlowTurn(ctx context.Context, f db.Flow, g flow.Graph, agent db.Agent, provider providers.Provider, req providers.Request, autonomous bool, onStep func(TurnStep)) (*providers.Response, []TurnStep, error) {
	input := flowInputText(req.Messages)
	run, err := r.db.CreateFlowRun(ctx, db.FlowRun{
		FlowID:    f.ID,
		AgentID:   agent.ID,
		SessionID: SessionIDFrom(ctx),
		MessageID: TurnIDFrom(ctx),
		Version:   f.Version,
		Trigger:   string(callKindFrom(ctx)),
		Input:     input,
	})
	if err != nil {
		// The store is the only thing that can fail here; a turn must not die
		// because its bookkeeping row could not be written.
		r.logger.Warn("flow: run row not created, running the plain turn", "flow", f.ID, "error", err)
		return r.executeTurn(ctx, agent, provider, req, autonomous, onStep)
	}
	r.emitFlowRunEvent(run)
	ctx = withFlowRunID(ctx, run.ID)

	runner := &turnFlowRunner{r: r, run: run, base: req, input: input, agent: agent, provider: provider, autonomous: autonomous, trivial: g.IsTrivial(), onStep: onStep}
	// Node cards are placed BEFORE the node's own steps, so the persisted trace
	// reads stage by stage; the live card with the same id is replaced in place
	// when the node finishes.
	cardIndex := map[string]int{}
	started := time.Now()
	res := flow.Run(ctx, g, input, runner, func(ev flow.Event) {
		r.emitFlowNodeEvent(run, ev)
		if runner.trivial {
			return
		}
		key := fmt.Sprintf("%s#%d", ev.NodeID, ev.Visit)
		card := flowNodeStep(run.ID, ev)
		switch ev.Phase {
		case "start":
			cardIndex[key] = len(runner.steps)
			runner.steps = append(runner.steps, card)
		default:
			if i, ok := cardIndex[key]; ok && i < len(runner.steps) {
				runner.steps[i] = card
			} else {
				runner.steps = append(runner.steps, card)
			}
		}
		if onStep != nil {
			onStep(card)
		}
	})
	dur := time.Since(started).Milliseconds()
	status, errText := db.FlowSuccess, ""
	if res.Err != nil {
		status, errText = db.FlowFailure, res.Err.Error()
	}
	stepsJSON, _ := json.Marshal(res.Steps)
	usage := db.FlowRunUsage{InputTokens: int64(runner.usage.InputTokens), OutputTokens: int64(runner.usage.OutputTokens), LLMCalls: runner.providerCalls}
	if finished, ferr := r.db.FinishFlowRun(ctx, run.ID, status, res.Output, errText, stepsJSON, len(res.Steps), dur, usage); ferr == nil {
		run = finished
	} else {
		r.logger.Warn("flow: finish run failed", "run", run.ID, "error", ferr)
	}
	r.emitFlowRunEvent(run)
	// Grading, flow-kind automations and the observer run detached; the
	// reply does not wait for any of them.
	r.afterFlowRun(ctx, f, run, agent)
	if res.Err != nil {
		return nil, runner.steps, res.Err
	}
	resp := &providers.Response{
		Text:          res.Output,
		StopReason:    runner.lastStop,
		Usage:         runner.usage,
		Model:         runner.lastModel,
		SessionID:     runner.threadSessionID,
		ProviderCalls: runner.providerCalls,
	}
	if resp.StopReason == "" {
		resp.StopReason = providers.StopEndTurn
	}
	if last := runner.lastResp; last != nil {
		// Carry the per-turn signals the chat path reads off the response:
		// container/compaction/first-call prompt measurement all describe the
		// last real model call.
		resp.ContainerID = last.ContainerID
		resp.NativeCompactionError = last.NativeCompactionError
		resp.FirstCallPromptTokens = last.FirstCallPromptTokens
		if last.Model != "" {
			resp.Model = last.Model
		}
	}
	return resp, runner.steps, nil
}

// flowInputText is the turn's input: the newest user message's text.
func flowInputText(msgs []providers.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == providers.RoleUser {
			return msgs[i].Text
		}
	}
	return ""
}

// flowNodeStep renders one node lifecycle frame as a trace step card.
func flowNodeStep(runID string, ev flow.Event) TurnStep {
	title := ev.Title
	if title == "" {
		title = ev.NodeID
	}
	st := TurnStep{
		Kind:       StepFlowNode,
		ID:         fmt.Sprintf("fn:%s:%s:%d", runID, ev.NodeID, ev.Visit),
		Text:       title,
		Target:     []string{ev.Type, ev.NodeID},
		Status:     ev.Phase,
		DurationMs: ev.DurationMs,
		Reason:     flowNodeReason(ev),
	}
	switch ev.Phase {
	case "start":
		st.Running = true
		st.Status = "running"
	case "done":
		st.Output = truncateRunes(ev.Output, flowNodeOutputExcerpt)
	case "error":
		st.IsError = true
		st.Output = ev.Error
	}
	return st
}

// flowNodeReason is the card's one-line verdict: the arm a route took and/or
// the detail (criteria verdicts, what a trigger launched).
func flowNodeReason(ev flow.Event) string {
	switch {
	case ev.Edge != "" && ev.Detail != "":
		return ev.Edge + " · " + ev.Detail
	case ev.Detail != "":
		return ev.Detail
	}
	return ev.Edge
}

// emitFlowNodeEvent broadcasts one live node frame (flow_node) keyed by run.
func (r *Runtime) emitFlowNodeEvent(run db.FlowRun, ev flow.Event) {
	if r == nil || r.bus == nil {
		return
	}
	if ev.Output != "" {
		ev.Output = truncateRunes(ev.Output, flowNodeOutputExcerpt)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeFlowNode,
		Level:  "info",
		Target: map[string]string{"flowRunId": run.ID, "flowId": run.FlowID, "agentId": run.AgentID, "sessionId": run.SessionID},
		Node:   raw,
	})
}

// emitFlowChanged raises the flow-screen refresh signal (events.TypeFlow) with
// a short notice, e.g. after a version commit or a filed proposal.
func (r *Runtime) emitFlowChanged(flowID, title, body string) {
	if r == nil || r.bus == nil {
		return
	}
	r.publish(events.Event{
		Type:   events.TypeFlow,
		Level:  "info",
		Title:  title,
		Body:   body,
		Target: map[string]string{"view": "flows", "flowId": flowID},
	})
}

// StartFlowRunSweeper prunes finished runs beyond the configured retention on a
// slow tick. Not registered with the shutdown barrier (an endless ticker); each
// tick checks the barrier so no write lands on a closing store.
func (r *Runtime) StartFlowRunSweeper(ctx context.Context) {
	go func() {
		t := time.NewTicker(waitingSweepInterval * 20)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if r.backgroundTurnsClosing() {
					return
				}
				keep := 0
				if r.tun != nil {
					keep = r.tun.FlowRunRetention()
				}
				if keep <= 0 {
					continue
				}
				if n, err := r.db.PruneFlowRuns(ctx, keep); err != nil {
					r.logger.Warn("flow: prune runs failed", "error", err)
				} else if n > 0 {
					r.logger.Info("flow: pruned finished runs", "removed", n, "keep", keep)
				}
			}
		}
	}()
}

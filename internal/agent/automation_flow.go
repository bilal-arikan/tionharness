package agent

import (
	"context"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
)

// Flow-kind automations (_Docs/93): rules that fire when a flow run finishes,
// narrowed by the flow's agent, the run's outcome and the decision model's
// grade. Delivered through Runtime.FireFlowRunFinished after grading, so a
// rule like "graded 2 or below → spawn the reviewer" sees the grade.

// OnFlowRunFinished is the flow-run hook: it matches every enabled flow rule
// against the finished run and fires the ones that apply. A run whose session
// this very rule spawned does not re-fire it, so a rule targeting the same
// agent cannot chain itself forever (MaxIterations still bounds the rest).
func (e *AutomationEngine) OnFlowRunFinished(ctx context.Context, ev FlowRunFinished) {
	run := ev.Run
	if run.Status == db.FlowRunning {
		return
	}
	autos, err := e.db.ListEnabledAutomations(ctx)
	if err != nil {
		e.logger.Warn("automation: list failed (flow)", "run", run.ID, "error", err)
		return
	}
	for _, a := range autos {
		if a.TriggerKind != db.TriggerFlow || !flowRuleMatches(a, run) {
			continue
		}
		if e.spawnedByRule(ctx, a, run.SessionID) {
			e.logger.Info("automation: flow rule skipped its own session", "automation", a.ID, "run", run.ID, "session", run.SessionID)
			continue
		}
		e.fireFlow(ctx, a, run)
	}
}

// flowRuleMatches applies a flow rule's filters to a finished run.
func flowRuleMatches(a db.Automation, run db.FlowRun) bool {
	if a.FlowAgentID != "" && a.FlowAgentID != run.AgentID {
		return false
	}
	if a.FlowStatus != "" && a.FlowStatus != run.Status {
		return false
	}
	if a.FlowMaxGrade > 0 && (run.Grade == 0 || run.Grade > a.FlowMaxGrade) {
		return false
	}
	return true
}

// spawnedByRule reports whether sessionID was itself started by automation a.
func (e *AutomationEngine) spawnedByRule(ctx context.Context, a db.Automation, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	sess, err := e.db.GetSession(ctx, sessionID)
	if err != nil || sess.Origin == nil {
		return false
	}
	return sess.Origin.Kind == db.OriginAutomation && sess.Origin.EntityID == a.ID
}

// fireFlow evaluates one matching flow rule's guardrails and, if they pass,
// dispatches it with the run rendered into the prompt.
func (e *AutomationEngine) fireFlow(ctx context.Context, a db.Automation, run db.FlowRun) {
	if !e.guardsPass(ctx, a) {
		return
	}
	prompt := renderAutomationPrompt(a.PromptTemplate, e.flowVars(ctx, a, run, "", ""))
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		return
	}
	sessionID, driver, err := e.dispatchFire(ctx, a, prompt, TriggerAutomationFlow, SpawnOptions{
		Title:           "🧬 " + automationLabel(a),
		CreatedBy:       "automation:" + a.ID,
		ParentSessionID: run.SessionID,
		Tags:            a.SpawnTags,
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return
	}
	e.logger.Info("automation: fired (flow)",
		"automation", a.ID, "run", run.ID, "status", run.Status, "grade", run.Grade,
		"session", sessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, sessionID, "🧬", " (akış koşusu)", prompt)
}

// fireFromFlowNode fires automation a from a trigger node of a running flow.
// It returns the launched session id, "" when a guard declined (recorded in
// the rule's ledger), or an error when the dispatch itself failed.
func (e *AutomationEngine) fireFromFlowNode(ctx context.Context, a db.Automation, run db.FlowRun, node flow.Node, payload string) (string, error) {
	if reason := e.guardReason(ctx, a); reason != "" {
		e.recordSkip(ctx, a, reason)
		return "", nil
	}
	prompt := renderAutomationPrompt(a.PromptTemplate, e.flowVars(ctx, a, run, node.ID, payload))
	if strings.TrimSpace(prompt) == "" {
		prompt = payload
	}
	sessionID, driver, err := e.dispatchFire(ctx, a, prompt, TriggerFlowNode, SpawnOptions{
		Title:           "⚡ " + automationLabel(a),
		CreatedBy:       "flow:" + run.FlowID,
		ParentSessionID: run.SessionID,
		Tags:            a.SpawnTags,
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return "", err
	}
	e.logger.Info("automation: fired (flow node)",
		"automation", a.ID, "run", run.ID, "node", node.ID, "session", sessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, sessionID, "⚡", " (akış düğümü)", prompt)
	return sessionID, nil
}

// flowVars assembles the placeholder values a flow-fired automation's prompt
// template may use. {{result}} is the trigger node's rendered payload, else
// the run's output (its error text for a failed run).
func (e *AutomationEngine) flowVars(ctx context.Context, a db.Automation, run db.FlowRun, nodeID, payload string) map[string]string {
	agentName := run.AgentID
	if ag, err := e.db.GetAgent(ctx, run.AgentID); err == nil {
		agentName = ag.Name
	}
	flowName := run.FlowID
	if f, err := e.db.GetFlow(ctx, run.FlowID); err == nil {
		flowName = f.Name
	}
	result := payload
	if result == "" {
		result = run.Output
		if result == "" && run.Status == db.FlowFailure {
			result = run.Error
		}
	}
	grade := ""
	if run.Grade > 0 {
		grade = strconv.Itoa(run.Grade)
	}
	v := commonVars(a)
	v["result"] = result
	v["input"] = run.Input
	v["output"] = run.Output
	v["error"] = run.Error
	v["status"] = run.Status
	v["grade"] = grade
	v["runId"] = run.ID
	v["flowId"] = run.FlowID
	v["flowName"] = flowName
	v["title"] = flowName
	v["version"] = strconv.Itoa(run.Version)
	v["agent"] = agentName
	v["agentName"] = agentName
	v["agentId"] = run.AgentID
	v["sessionId"] = run.SessionID
	v["nodeId"] = nodeID
	return v
}

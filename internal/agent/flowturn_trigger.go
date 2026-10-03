package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/flow"
)

// Trigger nodes (_Docs/93): a flow may fire an automation from inside a run —
// "after the answer, hand it to the reviewer automation", "when the gate
// fails, open a repair session". The node renders its payload template, the
// automation's own prompt template sees it as {{result}} (plus the flow
// variables, see flowVars) and the automation's guards (enabled, cooldown,
// iteration cap, autonomy brake) apply as for any other fire. The flow's last
// output passes through untouched, and a fire that cannot happen is recorded
// on the step rather than failing the user's turn.

// Trigger implements flow.Runner.
func (t *turnFlowRunner) Trigger(ctx context.Context, node flow.Node, payload string) (string, error) {
	return t.r.fireFlowNodeAutomation(ctx, t.run, node, payload), nil
}

// fireFlowNodeAutomation fires node.AutomationID for run and returns the step
// detail: what was launched, or why nothing was.
func (r *Runtime) fireFlowNodeAutomation(ctx context.Context, run db.FlowRun, node flow.Node, payload string) string {
	a, err := r.db.GetAutomation(ctx, node.AutomationID)
	if err != nil {
		r.logger.Warn("flow trigger: automation missing", "run", run.ID, "node", node.ID, "automation", node.AutomationID, "error", err)
		return fmt.Sprintf("skipped: automation %s not found", node.AutomationID)
	}
	if !a.Enabled || a.Archived {
		return fmt.Sprintf("skipped: automation %s is disabled", automationLabel(a))
	}
	e := NewAutomationEngine(r.db, r, r.logger)
	sessionID, err := e.fireFromFlowNode(ctx, a, run, node, payload)
	if err != nil {
		return "failed: " + err.Error()
	}
	if sessionID == "" {
		return "skipped: " + automationLabel(a) + " guards"
	}
	return fmt.Sprintf("triggered %s (%s) → session %s", automationLabel(a), a.ID, sessionID)
}

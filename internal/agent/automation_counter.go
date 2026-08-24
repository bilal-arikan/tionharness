package agent

import (
	"context"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// ActivityRecorded is the signal a message append delivers to the automation
// engine so a counter-triggered automation can detect a count crossing. It mirrors
// db.ActivitySignal (the store-layer type) — kept as a separate agent-layer type
// so the engine's hook signature does not leak the db type into every caller.
// Emitted from the store's activity hook, wired by the workspace manager.
type ActivityRecorded struct {
	SessionID    string
	MessageTotal int
	MessageDelta int
	ToolTotal    int
	ToolDelta    int
}

// OnActivityRecorded is the activity hook. It fires after every message append
// and, for each enabled counter automation, checks whether the append pushed the
// watched session counter across another CounterInterval multiple. The crossing is
// detected statelessly from the previous vs new total (new − delta = previous), so
// no per-scope ledger is kept — the same technique the token path uses. Runs on
// its own detached goroutine (wired by the manager) so appends are never blocked.
//
// Counter automations are session-scoped: a rule watches the counter of the
// session whose activity crossed the boundary. As with the token path, crossings
// attributed to an automation's own maintenance session are skipped so a rule's
// upkeep messages cannot re-fire it on a tight loop.
func (e *AutomationEngine) OnActivityRecorded(ctx context.Context, sig ActivityRecorded) {
	if sig.SessionID == "" {
		return // no session to attribute the crossing to
	}
	if sig.MessageDelta <= 0 && sig.ToolDelta <= 0 {
		return // nothing moved → no boundary can be crossed
	}
	autos, err := e.db.ListEnabledAutomations(ctx)
	if err != nil {
		e.logger.Warn("automation: list failed (counter)", "session", sig.SessionID, "error", err)
		return
	}
	// Self-amplification guard, resolved lazily and only when a counter rule is
	// actually present: a counter automation delivers into its OWN persistent
	// maintenance session (SessionKindAutomation), whose reply messages/tool calls
	// would push the watched counter across the next interval and re-fire the rule.
	// Applied to BOTH scopes (unlike the token workspace scope, which counts its
	// upkeep as real spend): a maintenance turn reliably adds messages+tools every
	// fire, so letting it self-trigger would loop tightly. The upkeep still COUNTS
	// toward the workspace total — it just does not itself cause a fire.
	crossingIsMaint := false
	maintResolved := false
	isMaintSession := func() bool {
		if !maintResolved {
			maintResolved = true
			if s, err := e.db.GetSession(ctx, sig.SessionID); err == nil {
				crossingIsMaint = s.Kind == SessionKindAutomation
			}
		}
		return crossingIsMaint
	}
	// Workspace-scope totals resolved once, lazily: only summed if some enabled
	// counter automation actually watches the workspace scope (the token path's
	// wsTotal pattern). -1 = not yet resolved.
	wsMsg, wsTool := int64(-1), int64(-1)
	for _, a := range autos {
		if a.TriggerKind != db.TriggerCounter || a.CounterInterval <= 0 {
			continue
		}
		if isMaintSession() {
			continue
		}
		interval := int64(a.CounterInterval)
		// delta is this append's contribution to the watched metric (same for both
		// scopes); total is the post-append cumulative total at the watched scope.
		var total, delta int64
		metric := a.CounterMetric
		switch metric {
		case db.CounterMetricTool:
			delta = int64(sig.ToolDelta)
		default: // "" or CounterMetricMessage
			delta = int64(sig.MessageDelta)
		}
		if delta <= 0 {
			continue // this append did not move the watched metric
		}
		switch a.CounterScope {
		case db.CounterScopeWorkspace:
			if metric == db.CounterMetricTool {
				if wsTool < 0 {
					wsTool = e.db.WorkspaceCounterTotal(db.CounterMetricTool)
				}
				total = wsTool
			} else {
				if wsMsg < 0 {
					wsMsg = e.db.WorkspaceCounterTotal(db.CounterMetricMessage)
				}
				total = wsMsg
			}
			if crossedMultiple(total-delta, total, interval) {
				e.fireCounter(ctx, a, "", total) // no single crossing session for workspace scope
			}
		default: // "" or CounterScopeSession
			if metric == db.CounterMetricTool {
				total = int64(sig.ToolTotal)
			} else {
				total = int64(sig.MessageTotal)
			}
			if crossedMultiple(total-delta, total, interval) {
				e.fireCounter(ctx, a, sig.SessionID, total)
			}
		}
	}
}

// fireCounter evaluates a counter automation's shared guardrails and, if they
// pass, runs its target flow or spawns its target agent with the counter context
// in the prompt. It mirrors fireToken: delivery continues the automation's own
// persistent maintenance session so successive crossings extend one thread.
func (e *AutomationEngine) fireCounter(ctx context.Context, a db.Automation, sessionID string, total int64) {
	if !e.guardsPass(ctx, a) {
		return
	}
	prompt := renderAutomationPrompt(a.PromptTemplate, e.counterVars(a, sessionID, total))
	if strings.TrimSpace(prompt) == "" {
		e.recordFailure(ctx, a, "rendered prompt is empty")
		return
	}
	firedSessionID, driver, err := e.dispatchFire(ctx, a, prompt, TriggerAutomationCounter, SpawnOptions{
		Title:     "⚡ " + automationLabel(a),
		CreatedBy: "automation:" + a.ID,
		Tags:      a.SpawnTags, // counter rules do not self-loop; nil = no tag
	})
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return
	}
	metric := counterMetricLabel(a.CounterMetric)
	scope := a.EffectiveCounterScope()
	suffix := " (counter·" + scope + "·" + metric + ")"
	if driver == "flow" {
		suffix = " (counter·" + scope + "·" + metric + "·akış)"
	}
	e.logger.Info("automation: fired (counter)",
		"automation", a.ID, "scope", scope, "metric", metric, "total", total, "interval", a.CounterInterval,
		"session", firedSessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.notifyFired(ctx, a, firedSessionID, "⚡", suffix, prompt)
}

// counterVars assembles the placeholder values for a counter automation's prompt.
// There is no session result, so {{result}} is absent (nothing is appended).
func (e *AutomationEngine) counterVars(a db.Automation, sessionID string, total int64) map[string]string {
	v := commonVars(a)
	v["count"] = strconv.FormatInt(total, 10)
	v["metric"] = counterMetricLabel(a.CounterMetric)
	v["scope"] = a.EffectiveCounterScope()
	v["interval"] = strconv.Itoa(a.CounterInterval)
	v["sessionId"] = sessionID // empty for workspace scope
	return v
}

// counterMetricLabel normalizes an empty metric to the message default for
// display and prompt substitution.
func counterMetricLabel(metric string) string {
	if metric == "" {
		return db.CounterMetricMessage
	}
	return metric
}

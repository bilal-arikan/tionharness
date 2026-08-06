package agent

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
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
	// maintenance session (SessionKindAutomation), whose reply messages would push
	// that same session across the next interval and re-fire the rule.
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
	for _, a := range autos {
		if a.TriggerKind != db.TriggerCounter || a.CounterInterval <= 0 {
			continue
		}
		if isMaintSession() {
			continue
		}
		interval := int64(a.CounterInterval)
		var total, delta int64
		switch a.CounterMetric {
		case db.CounterMetricTool:
			total, delta = int64(sig.ToolTotal), int64(sig.ToolDelta)
		default: // "" or CounterMetricMessage
			total, delta = int64(sig.MessageTotal), int64(sig.MessageDelta)
		}
		if delta <= 0 {
			continue // this append did not move the watched metric
		}
		if crossedMultiple(total-delta, total, interval) {
			e.fireCounter(ctx, a, sig.SessionID, total)
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

	var (
		firedSessionID string
		driver         string
		err            error
	)
	if a.FlowID != "" {
		var res LaunchResult
		res, err = e.rt.LaunchRun(ctx, RunSpec{
			Trigger:    TriggerAutomationCounter,
			Input:      prompt,
			Autonomous: true,
			FlowID:     a.FlowID,
		})
		firedSessionID, driver = res.SessionID, "flow"
	} else {
		// The launch brake is bypassed on the reuse path, so honor the workspace
		// autonomy pause here before delivering an autonomous turn.
		if e.rt.Paused() {
			err = ErrAutonomyPaused
		} else {
			firedSessionID, err = e.rt.deliverAutomationTurn(ctx, a, prompt)
			driver = "session"
		}
	}
	if err != nil {
		e.recordFailure(ctx, a, err.Error())
		return
	}
	if err := e.db.RecordAutomationFire(ctx, a.ID, firedSessionID, ""); err != nil {
		e.logger.Warn("automation: record fire failed", "automation", a.ID, "error", err)
	}
	metric := counterMetricLabel(a.CounterMetric)
	suffix := " (counter·" + metric + ")"
	if driver == "flow" {
		suffix = " (counter·" + metric + "·akış)"
	}
	e.logger.Info("automation: fired (counter)",
		"automation", a.ID, "metric", metric, "total", total, "interval", a.CounterInterval,
		"session", firedSessionID, "driver", driver, "iteration", a.IterationCount+1)
	e.rt.publish(events.Event{
		Type:   events.TypeAutomation,
		Level:  "success",
		Title:  "⚡ Otomasyon tetiklendi" + suffix + " — " + automationLabel(a),
		Body:   notifyLine(prompt, 120),
		Target: map[string]string{"view": "executions", "sessionId": firedSessionID},
	})
}

// counterVars assembles the placeholder values for a counter automation's prompt.
// There is no session result, so {{result}} is absent (nothing is appended).
func (e *AutomationEngine) counterVars(a db.Automation, sessionID string, total int64) map[string]string {
	now := time.Now()
	maxIter := strconv.Itoa(a.MaxIterations)
	if a.MaxIterations == 0 {
		maxIter = "∞"
	}
	metric := counterMetricLabel(a.CounterMetric)
	return map[string]string{
		"count":         strconv.FormatInt(total, 10),
		"metric":        metric,
		"interval":      strconv.Itoa(a.CounterInterval),
		"sessionId":     sessionID,
		"iteration":     strconv.Itoa(a.IterationCount + 1),
		"maxIterations": maxIter,
		"automation":    automationLabel(a),
		"date":          now.Format("2006-01-02"),
		"time":          now.Format("15:04"),
		"datetime":      now.Format("2006-01-02 15:04"),
	}
}

// counterMetricLabel normalizes an empty metric to the message default for
// display and prompt substitution.
func counterMetricLabel(metric string) string {
	if metric == "" {
		return db.CounterMetricMessage
	}
	return metric
}

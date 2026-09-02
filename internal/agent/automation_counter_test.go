package agent

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestCounterVarsSubstitution(t *testing.T) {
	e := &AutomationEngine{}
	a := db.Automation{TriggerKind: db.TriggerCounter, CounterMetric: db.CounterMetricTool, CounterInterval: 10, MaxIterations: 5}
	vars := e.counterVars(a, "SES7", 30)
	got := renderAutomationPrompt("{{metric}} {{scope}} {{sessionId}} {{count}}/{{interval}} #{{iteration}}", vars)
	want := "tool session SES7 30/10 #1"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Workspace scope renders {{scope}}=workspace with an empty sessionId.
	aw := db.Automation{TriggerKind: db.TriggerCounter, CounterScope: db.CounterScopeWorkspace, CounterInterval: 150}
	if v := renderAutomationPrompt("{{scope}}|{{sessionId}}", e.counterVars(aw, "", 300)); v != "workspace|" {
		t.Fatalf("workspace vars render = %q", v)
	}
	// An empty metric renders as the "message" default.
	am := db.Automation{TriggerKind: db.TriggerCounter, CounterInterval: 5}
	if v := renderAutomationPrompt("{{metric}}", e.counterVars(am, "SES1", 5)); v != "message" {
		t.Fatalf("empty metric must default to message, got %q", v)
	}
}

func TestDurableCounterInboxUsesCapturedWorkspaceTotalsOnce(t *testing.T) {
	e := backstopEngine(t)
	a := seedAutomation(t, e, db.Automation{
		TriggerKind: db.TriggerCounter, CounterScope: db.CounterScopeWorkspace,
		CounterInterval: 5, MaxIterations: 10, TargetAgentID: "missing-agent",
	})
	for _, sig := range []db.ActivitySignal{
		{EventID: "event-at-threshold", SessionID: "source", MessageTotal: 1, MessageDelta: 1, WorkspaceMessageTotal: 5},
		{EventID: "event-after-threshold", SessionID: "source", MessageTotal: 2, MessageDelta: 1, WorkspaceMessageTotal: 6},
	} {
		if accepted, err := e.db.AcceptActivitySignal(sig); err != nil || !accepted {
			t.Fatalf("accept %s = %v, %v", sig.EventID, accepted, err)
		}
	}
	if err := e.DrainActivityInbox(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := e.db.GetAutomation(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.IterationCount != 1 {
		t.Fatalf("fires after first drain = %d", got.IterationCount)
	}
	if err := e.DrainActivityInbox(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err = e.db.GetAutomation(context.Background(), a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.IterationCount != 1 {
		t.Fatalf("completed receipts replayed, fires = %d", got.IterationCount)
	}
}

func TestCounterMetricLabel(t *testing.T) {
	if counterMetricLabel("") != db.CounterMetricMessage {
		t.Errorf("empty metric must default to %q", db.CounterMetricMessage)
	}
	if counterMetricLabel(db.CounterMetricTool) != db.CounterMetricTool {
		t.Errorf("explicit tool metric must pass through")
	}
}

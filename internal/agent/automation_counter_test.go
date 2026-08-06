package agent

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
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

func TestCounterMetricLabel(t *testing.T) {
	if counterMetricLabel("") != db.CounterMetricMessage {
		t.Errorf("empty metric must default to %q", db.CounterMetricMessage)
	}
	if counterMetricLabel(db.CounterMetricTool) != db.CounterMetricTool {
		t.Errorf("explicit tool metric must pass through")
	}
}

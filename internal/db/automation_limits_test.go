package db

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

// TestValidateMaxIterations locks the boundary table for the guard that was
// documented and shipped in the UI on 2026-07-29 but never actually enforced: the
// API accepted 0 without complaint, and four stored automations still carried it
// (three of them enabled and firing). A UI-only rule is not a rule — both the REST
// handler and the agent tool reach this function.
func TestValidateMaxIterations(t *testing.T) {
	rejected := map[int]string{
		0:                        "0 is the old unlimited value — the whole point of the guard",
		-1:                       "negative would also read as unlimited",
		-100:                     "same",
		MaxIterationsHardCap + 1: "one past the ceiling",
		1_000_000:                "the fat-finger case the ceiling exists for",
	}
	for v, why := range rejected {
		err := ValidateMaxIterations(v)
		if err == nil {
			t.Errorf("maxIterations=%d must be rejected (%s)", v, why)
			continue
		}
		// Callers branch on the sentinel; the wrapped text is for the human.
		if !errors.Is(err, ErrMaxIterationsRange) {
			t.Errorf("maxIterations=%d must wrap ErrMaxIterationsRange, got %v", v, err)
		}
	}

	for _, v := range []int{1, 10, 50, MaxIterationsHardCap} {
		if err := ValidateMaxIterations(v); err != nil {
			t.Errorf("maxIterations=%d must be accepted, got %v", v, err)
		}
	}

	// The message has to name the limit — "invalid value" would leave the user
	// guessing at a number only the server knows.
	if err := ValidateMaxIterations(MaxIterationsHardCap + 1); !strings.Contains(err.Error(), strconv.Itoa(MaxIterationsHardCap)) {
		t.Errorf("the ceiling message must state the ceiling, got %q", err)
	}
	if err := ValidateMaxIterations(0); !strings.Contains(err.Error(), "0") {
		t.Errorf("the zero message must explain what is wrong with 0, got %q", err)
	}
}

// TestValidateTokenThreshold locks the floor for a token automation's interval.
// A tiny interval would cross on nearly every call and fire in a tight loop, so
// the same validator guards both the REST handler and the agent tool.
func TestValidateTokenThreshold(t *testing.T) {
	for _, v := range []int{0, 1, MinTokenThreshold - 1, -100} {
		err := ValidateTokenThreshold(v)
		if err == nil {
			t.Errorf("tokenThreshold=%d must be rejected (below floor %d)", v, MinTokenThreshold)
			continue
		}
		if !errors.Is(err, ErrTokenThresholdRange) {
			t.Errorf("tokenThreshold=%d must wrap ErrTokenThresholdRange, got %v", v, err)
		}
	}
	for _, v := range []int{MinTokenThreshold, 100_000, 5_000_000} {
		if err := ValidateTokenThreshold(v); err != nil {
			t.Errorf("tokenThreshold=%d must be accepted, got %v", v, err)
		}
	}
	// The message must name the floor so the user knows the minimum.
	if err := ValidateTokenThreshold(1); !strings.Contains(err.Error(), strconv.Itoa(MinTokenThreshold)) {
		t.Errorf("the floor message must state the floor, got %q", err)
	}
}

// TestValidateCounterInterval locks the floor for a counter automation's interval.
// An interval of 1 would fire on nearly every append, so the same validator guards
// both the REST handler and the agent tool.
func TestValidateCounterInterval(t *testing.T) {
	for _, v := range []int{0, 1, MinCounterInterval - 1, -5} {
		err := ValidateCounterInterval(v)
		if err == nil {
			t.Errorf("counterInterval=%d must be rejected (below floor %d)", v, MinCounterInterval)
			continue
		}
		if !errors.Is(err, ErrCounterIntervalRange) {
			t.Errorf("counterInterval=%d must wrap ErrCounterIntervalRange, got %v", v, err)
		}
	}
	for _, v := range []int{MinCounterInterval, 10, 100} {
		if err := ValidateCounterInterval(v); err != nil {
			t.Errorf("counterInterval=%d must be accepted, got %v", v, err)
		}
	}
}

// TestValidCounterMetric pins the accepted metric set (empty defaults to message).
func TestValidCounterMetric(t *testing.T) {
	for _, m := range []string{"", CounterMetricMessage, CounterMetricTool} {
		if !ValidCounterMetric(m) {
			t.Errorf("metric %q must be valid", m)
		}
	}
	for _, m := range []string{"messages", "step", "bogus"} {
		if ValidCounterMetric(m) {
			t.Errorf("metric %q must be invalid", m)
		}
	}
}

// TestValidCounterScope pins the accepted scope set (empty defaults to session).
func TestValidCounterScope(t *testing.T) {
	for _, s := range []string{"", CounterScopeSession, CounterScopeWorkspace} {
		if !ValidCounterScope(s) {
			t.Errorf("scope %q must be valid", s)
		}
	}
	for _, s := range []string{"daily", "agent", "bogus"} {
		if ValidCounterScope(s) {
			t.Errorf("scope %q must be invalid", s)
		}
	}
}

// TestValidSessionMode pins the accepted mode set (empty resolves per kind).
func TestValidSessionMode(t *testing.T) {
	for _, m := range []string{"", SessionModeSpawn, SessionModeContinue} {
		if !ValidSessionMode(m) {
			t.Errorf("mode %q must be valid", m)
		}
	}
	for _, m := range []string{"new", "reuse", "bogus"} {
		if ValidSessionMode(m) {
			t.Errorf("mode %q must be invalid", m)
		}
	}
}

// TestEffectiveSessionMode locks the per-kind default resolution: an explicit mode
// always wins; empty falls back to continue for token/counter (the maintenance
// thread) and spawn for everything else — preserving pre-field behavior.
func TestEffectiveSessionMode(t *testing.T) {
	cases := []struct {
		kind, stored, want string
	}{
		{TriggerTag, "", SessionModeSpawn},
		{TriggerBoard, "", SessionModeSpawn},
		{TriggerToken, "", SessionModeContinue},
		{TriggerCounter, "", SessionModeContinue},
		{"", "", SessionModeSpawn},                             // legacy empty kind → tag → spawn
		{TriggerToken, SessionModeSpawn, SessionModeSpawn},     // explicit overrides default
		{TriggerTag, SessionModeContinue, SessionModeContinue}, // explicit overrides default
		{TriggerCounter, SessionModeSpawn, SessionModeSpawn},   // explicit overrides default
	}
	for _, c := range cases {
		got := Automation{TriggerKind: c.kind, SessionMode: c.stored}.EffectiveSessionMode()
		if got != c.want {
			t.Errorf("kind=%q stored=%q → %q, want %q", c.kind, c.stored, got, c.want)
		}
	}
}

// TestValidTokenScope pins the accepted scope set (empty defaults to session).
func TestValidTokenScope(t *testing.T) {
	for _, s := range []string{"", TokenScopeSession, TokenScopeWorkspace} {
		if !ValidTokenScope(s) {
			t.Errorf("scope %q must be valid", s)
		}
	}
	for _, s := range []string{"daily", "agent", "bogus"} {
		if ValidTokenScope(s) {
			t.Errorf("scope %q must be invalid", s)
		}
	}
}

// TestValidateAutomationShape locks the create/update-shared contract. The bug it
// closes: update_automation re-validated only the token threshold, so an agent
// could switch a rule's kind (or clear a field) into a state create rejects — a
// tag rule with no triggerTag (silently never fires) or a spawn rule with no
// target (fails only at fire time). Both write paths now run this.
func TestValidateAutomationShape(t *testing.T) {
	agent := "AGT1"

	valid := []Automation{
		{TriggerKind: TriggerTag, TriggerTag: "loop", TargetAgentID: agent, PromptTemplate: "run"},
		{TriggerKind: "", TriggerTag: "loop", FlowID: "FL1", PromptTemplate: "run"}, // "" == tag
		{TriggerKind: TriggerBoard, BoardAction: BoardActionArchive},                // archive needs no target
		{TriggerKind: TriggerBoard, BoardAction: BoardActionMove, BoardToState: "review", BoardMoveToState: "done"},
		{TriggerKind: TriggerBoard, BoardAction: "", TargetAgentID: agent, PromptTemplate: "run"}, // "" == spawn
		{TriggerKind: TriggerBoard, BoardAction: BoardActionSpawn, FlowID: "FL1", PromptTemplate: "run"},
		{TriggerKind: TriggerToken, TokenThreshold: MinTokenThreshold, TargetAgentID: agent, PromptTemplate: "run"},
		{TriggerKind: TriggerToken, TokenScope: TokenScopeWorkspace, TokenThreshold: 100_000, FlowID: "FL1", PromptTemplate: "run"},
		{TriggerKind: TriggerCounter, CounterInterval: MinCounterInterval, TargetAgentID: agent, PromptTemplate: "run"},
		{TriggerKind: TriggerCounter, CounterMetric: CounterMetricTool, CounterInterval: 10, FlowID: "FL1", PromptTemplate: "run"},
		{TriggerKind: TriggerCounter, CounterMetric: CounterMetricTool, CounterScope: CounterScopeWorkspace, CounterInterval: 150, TargetAgentID: agent, PromptTemplate: "run"},
		{TriggerKind: TriggerTag, TriggerTag: "loop", SessionMode: SessionModeContinue, TargetAgentID: agent, PromptTemplate: "run"},
		{TriggerKind: TriggerToken, TokenThreshold: MinTokenThreshold, SessionMode: SessionModeSpawn, TargetAgentID: agent, PromptTemplate: "run"},
	}
	for i, a := range valid {
		if err := ValidateAutomationShape(a); err != nil {
			t.Errorf("valid[%d] must pass, got %v", i, err)
		}
	}

	rejected := []struct {
		name string
		a    Automation
	}{
		{"tag without triggerTag never fires", Automation{TriggerKind: TriggerTag, TargetAgentID: agent}},
		{"legacy empty-kind without triggerTag", Automation{TriggerKind: "", TargetAgentID: agent}},
		{"tag with tag but no target", Automation{TriggerKind: TriggerTag, TriggerTag: "loop"}},
		{"board spawn without target", Automation{TriggerKind: TriggerBoard, BoardAction: BoardActionSpawn}},
		{"board spawn without prompt", Automation{TriggerKind: TriggerBoard, BoardAction: BoardActionSpawn, TargetAgentID: agent}},
		{"tag without prompt", Automation{TriggerKind: TriggerTag, TriggerTag: "loop", TargetAgentID: agent}},
		{"token without prompt", Automation{TriggerKind: TriggerToken, TokenThreshold: MinTokenThreshold, TargetAgentID: agent}},
		{"counter without prompt", Automation{TriggerKind: TriggerCounter, CounterInterval: MinCounterInterval, TargetAgentID: agent}},
		{"board move without destination", Automation{TriggerKind: TriggerBoard, BoardAction: BoardActionMove}},
		{"board move with wildcard destination filter self-triggers", Automation{TriggerKind: TriggerBoard, BoardAction: BoardActionMove, BoardMoveToState: "done"}},
		{"board move to matched destination self-triggers", Automation{TriggerKind: TriggerBoard, BoardAction: BoardActionMove, BoardToState: "done", BoardMoveToState: "done"}},
		{"board bad action", Automation{TriggerKind: TriggerBoard, BoardAction: "bogus", TargetAgentID: agent}},
		{"token without threshold", Automation{TriggerKind: TriggerToken, TargetAgentID: agent}},
		{"token bad scope", Automation{TriggerKind: TriggerToken, TokenScope: "daily", TokenThreshold: 100_000, TargetAgentID: agent}},
		{"token valid threshold but no target", Automation{TriggerKind: TriggerToken, TokenThreshold: 100_000}},
		{"counter without interval", Automation{TriggerKind: TriggerCounter, TargetAgentID: agent}},
		{"counter bad metric", Automation{TriggerKind: TriggerCounter, CounterMetric: "step", CounterInterval: 10, TargetAgentID: agent}},
		{"counter valid interval but no target", Automation{TriggerKind: TriggerCounter, CounterInterval: 10}},
		{"counter bad scope", Automation{TriggerKind: TriggerCounter, CounterScope: "daily", CounterInterval: 10, TargetAgentID: agent}},
		{"bad sessionMode", Automation{TriggerKind: TriggerTag, TriggerTag: "loop", SessionMode: "reuse", TargetAgentID: agent}},
	}
	for _, tc := range rejected {
		if err := ValidateAutomationShape(tc.a); err == nil {
			t.Errorf("%q must be rejected", tc.name)
		}
	}
}

func TestValidateAutomationShapePromptRequirements(t *testing.T) {
	tests := []Automation{
		{TriggerKind: TriggerBoard, BoardAction: BoardActionSpawn, TargetAgentID: "AGT1"},
		{TriggerKind: TriggerTag, TriggerTag: "loop", TargetAgentID: "AGT1"},
		{TriggerKind: TriggerToken, TokenThreshold: MinTokenThreshold, TargetAgentID: "AGT1"},
		{TriggerKind: TriggerCounter, CounterInterval: MinCounterInterval, TargetAgentID: "AGT1"},
	}
	for _, automation := range tests {
		err := ValidateAutomationShape(automation)
		if !errors.Is(err, ErrAutomationShape) {
			t.Fatalf("ValidateAutomationShape(%+v) error = %v, want ErrAutomationShape", automation, err)
		}
		if !strings.Contains(err.Error(), "promptTemplate is required") {
			t.Fatalf("ValidateAutomationShape(%+v) error = %q", automation, err)
		}
	}
}

// TestIterationLimitOrdering pins the relationship between the two constants.
// The backstop must sit ABOVE the hard cap: it exists for legacy rows nobody
// bounded, so tripping it earlier than an explicit maximum would punish exactly
// the data that never got a choice.
func TestIterationLimitOrdering(t *testing.T) {
	if AbsoluteIterationBackstop <= MaxIterationsHardCap {
		t.Fatalf("backstop (%d) must exceed the hard cap (%d)", AbsoluteIterationBackstop, MaxIterationsHardCap)
	}
	// And the cap must leave real headroom over the default (50), or the typo guard
	// doubles as a guard against ordinary configuration.
	if MaxIterationsHardCap < 50*5 {
		t.Errorf("hard cap (%d) leaves too little headroom over the default 50", MaxIterationsHardCap)
	}
}

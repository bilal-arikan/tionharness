package e2e

import (
	"strings"
	"testing"
)

// TestDelegation_DisabledToolAbsent confirms the gate: with delegation off
// (the default), run_subagent is not registered, so a model that tries to call it
// gets an "unknown tool" error result instead of spinning up a subagent.
func TestDelegation_DisabledToolAbsent(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Delegating.", tc("c1", "run_subagent", map[string]any{
			"target": "explore", "task": "look around",
		})),
		sayText("I'll handle it myself."),
	)
	h := newHarness(t, prov) // delegation defaults off
	ag := h.newAgent("Solo")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Delegate this to a subagent.")

	step := findToolStep(res.steps, "run_subagent")
	if step == nil {
		t.Fatalf("no run_subagent step in trace: %+v", res.steps)
	}
	if !step.IsError || !strings.Contains(step.Output, "unknown tool") {
		t.Errorf("expected unknown-tool error when delegation is off, got %q (err=%v)", step.Output, step.IsError)
	}
}

// TestDelegation_EnabledUnknownTargetErrors enables delegation and drives the
// run_subagent runner through its target-resolution guard: an unknown target
// (neither a built-in profile nor an existing agent) is rejected with a clear
// error fed back to the model.
func TestDelegation_EnabledUnknownTargetErrors(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Delegating to a ghost.", tc("c1", "run_subagent", map[string]any{
			"target": "ghost-agent", "task": "do something",
		})),
		sayText("That target did not exist."),
	)
	h := newHarness(t, prov)
	h.tun.SetDelegationEnabled(true)
	ag := h.newAgent("Director")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Send this to ghost-agent.")

	step := findToolStep(res.steps, "run_subagent")
	if step == nil {
		t.Fatalf("no run_subagent step in trace: %+v", res.steps)
	}
	if !step.IsError {
		t.Fatalf("expected an error for an unknown target, got: %q", step.Output)
	}
	if !strings.Contains(step.Output, "unknown subagent target") {
		t.Errorf("error should name the unknown target, got %q", step.Output)
	}

	// The turn still completes normally after the failed delegation.
	if res.resp.Text != "That target did not exist." {
		t.Errorf("final answer = %q", res.resp.Text)
	}
}

package e2e

import (
	"strings"
	"testing"
)

// TestDelegation_ToolAlwaysAvailable confirms run_subagent is always installed
// (2026-07-02: the delegation master toggle was removed; per-tool visibility from
// the Tools screen handles disabling). A call must reach the runner — NOT bounce
// with an "unknown tool" error. Targeting a non-existent agent lets us assert the
// tool ran (and reported target-not-found) without spinning a full subagent turn.
func TestDelegation_ToolAlwaysAvailable(t *testing.T) {
	prov := newScriptedProvider(
		callTools("Delegating.", tc("c1", "run_subagent", map[string]any{
			"target": "ghost-agent", "task": "look around",
		})),
		sayText("I'll handle it myself."),
	)
	h := newHarness(t, prov)
	ag := h.newAgent("Solo")
	sess := h.newSession(ag)

	res := h.send(ag, sess, "Delegate this to a subagent.")

	step := findToolStep(res.steps, "run_subagent")
	if step == nil {
		t.Fatalf("no run_subagent step in trace: %+v", res.steps)
	}
	if strings.Contains(step.Output, "unknown tool") {
		t.Errorf("run_subagent must always be registered, got unknown-tool error: %q", step.Output)
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

package agent

import "testing"

// TestParseStallVerdict pins the judge-reply parser: it must extract the boolean
// from strict or prose-wrapped JSON and fail SAFE (false) on anything garbled, so a
// malformed verdict never triggers a corrective nudge.
func TestParseStallVerdict(t *testing.T) {
	stalled := []string{
		`{"stalled": true}`,
		`{"stalled":true}`,
		"Here is my verdict:\n{\"stalled\": true}\n",
		"```json\n{\"stalled\": true}\n```",
	}
	for _, s := range stalled {
		if !parseStallVerdict(s) {
			t.Errorf("expected stalled=true for: %q", s)
		}
	}
	notStalled := []string{
		`{"stalled": false}`,
		"the coordinator concluded cleanly", // no JSON at all → false
		`{"other": true}`,                   // wrong key → false
		`{stalled: true}`,                   // invalid JSON → false
		"",                                  // empty → false
	}
	for _, s := range notStalled {
		if parseStallVerdict(s) {
			t.Errorf("did NOT expect stalled=true for: %q", s)
		}
	}
}

// TestSlotIsStallCandidate verifies the deterministic gate the sweeper applies
// before spending a judge call: only an idle coordinator that spawned workers, has
// none running now, and has been silent past the window is a candidate.
func TestSlotIsStallCandidate(t *testing.T) {
	const now = 1_000_000
	const window = 300 // 5 min

	// Happy path: idle, had workers, none running, silent 10 min.
	s := &coordSlot{hadWorkers: true, lastTurnUnix: now - 600}
	if !slotIsStallCandidate(s, false, now, window) {
		t.Error("expected a long-silent idle coordinator to be a candidate")
	}

	// Still running a turn → not a candidate.
	s = &coordSlot{driving: true, hadWorkers: true, lastTurnUnix: now - 600}
	if slotIsStallCandidate(s, false, now, window) {
		t.Error("a running coordinator must not be a candidate")
	}

	// Never spawned a worker and never seen by the guard as a coordinator → nothing
	// to reconcile. (A slot known to be in coordinator mode IS a candidate without
	// workers — see TestStallCandidateWithoutWorkers.)
	s = &coordSlot{hadWorkers: false, lastTurnUnix: now - 600}
	if slotIsStallCandidate(s, false, now, window) {
		t.Error("a slot with no worker history and no coordinator mode must not be a candidate")
	}

	// A worker is still running → legitimately waiting, not stalled.
	s = &coordSlot{hadWorkers: true, lastTurnUnix: now - 600}
	s.workers.Store(1)
	if slotIsStallCandidate(s, false, now, window) {
		t.Error("a coordinator with a running worker must not be a candidate")
	}

	// Silent for less than the window → too soon.
	s = &coordSlot{hadWorkers: true, lastTurnUnix: now - 60}
	if slotIsStallCandidate(s, false, now, window) {
		t.Error("a recently-active coordinator must not be a candidate")
	}

	// Never ran a real turn (lastTurnUnix == 0, e.g. a stubbed test) → excluded.
	s = &coordSlot{hadWorkers: true, lastTurnUnix: 0}
	if slotIsStallCandidate(s, false, now, window) {
		t.Error("a coordinator with no recorded turn must not be a candidate")
	}
}

// TestTurnCalledCoordinationTool verifies the tool-call detector matches both the bare
// native name and the namespaced claude-cli CallName, recurses into subagent steps,
// and stays false for an all-text turn (the hallucination case).
func TestTurnCalledCoordinationTool(t *testing.T) {
	if !turnCalledCoordinationTool([]TurnStep{{Kind: StepTool, Tool: "spawn_worker"}}) {
		t.Error("bare spawn_worker not detected")
	}
	if !turnCalledCoordinationTool([]TurnStep{{Kind: StepTool, CallName: "mcp__tionharness_interaction__list_workers"}}) {
		t.Error("namespaced list_workers not detected")
	}
	nested := []TurnStep{{Kind: StepText, Text: "delegating"}, {Kind: StepTool, Tool: "run_subagent", SubSteps: []TurnStep{{Kind: StepTool, Tool: "send_to_worker"}}}}
	if !turnCalledCoordinationTool(nested) {
		t.Error("nested send_to_worker not detected")
	}
	allText := []TurnStep{{Kind: StepText, Text: "3 worker başlattım [running]"}, {Kind: StepThinking, Text: "..."}}
	if turnCalledCoordinationTool(allText) {
		t.Error("all-text turn wrongly reported a coordination tool call")
	}
	unrelated := []TurnStep{{Kind: StepTool, Tool: "Bash"}, {Kind: StepTool, Tool: "Read"}}
	if turnCalledCoordinationTool(unrelated) {
		t.Error("unrelated tools wrongly matched")
	}
}

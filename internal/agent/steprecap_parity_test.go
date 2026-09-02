package agent

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestRecapStepParity pins the JSON contract between agent.TurnStep (the writer)
// and tools.RecapStep (the reader used by the prompt recap and get_session_info).
// The reader cannot import the writer — package agent depends on package tools,
// not the other way round — so a renamed tag on TurnStep would not break the
// build: it would silently decode into a zero value and every recap would go
// quietly blank. This test is that missing compile error.
func TestRecapStepParity(t *testing.T) {
	written := TurnStep{
		Kind:     StepTool,
		Tool:     "Bash",
		CallName: "mcp__tionharness_interaction__Bash",
		Input:    json.RawMessage(`{"command":"go test ./..."}`),
		Output:   "ok",
		IsError:  true,
		Reason:   "permission_denied",
		Running:  true,
	}
	raw, err := json.Marshal([]TurnStep{written})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	read, err := tools.ParseRecapSteps(string(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(read) != 1 {
		t.Fatalf("expected 1 step, got %d", len(read))
	}
	got := read[0]
	if got.Kind != string(written.Kind) {
		t.Errorf("kind: got %q want %q", got.Kind, written.Kind)
	}
	if got.Tool != written.Tool || got.CallName != written.CallName {
		t.Errorf("names: got %q/%q want %q/%q", got.Tool, got.CallName, written.Tool, written.CallName)
	}
	if string(got.Input) != string(written.Input) {
		t.Errorf("input: got %s want %s", got.Input, written.Input)
	}
	if got.Output != written.Output || !got.IsError || !got.Running {
		t.Errorf("output/flags mismatch: %+v", got)
	}
	if got.Reason != written.Reason {
		t.Errorf("reason: got %q want %q", got.Reason, written.Reason)
	}
}

// TestRecapStepParityKindNames pins the step-kind STRINGS tools.RecapErrors and
// RecapStep.isToolCall match on. They are compared as literals over there, so a
// changed constant value here must fail loudly rather than turn the classifier
// into a no-op.
func TestRecapStepParityKindNames(t *testing.T) {
	for _, tc := range []struct {
		kind StepKind
		want string
	}{
		{StepTool, "tool"},
		{StepDiff, "diff"},
		{StepError, "error"},
		{StepRecovery, "recovery"},
	} {
		if string(tc.kind) != tc.want {
			t.Errorf("step kind drifted: %q is now %q — update tools/steprecap.go", tc.want, tc.kind)
		}
	}
}

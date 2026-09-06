package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestSteerableForTurn pins the rule that gates the "unsupported" steer response:
// native providers steer in any mode; claude-cli only in "ask" (the one mode whose
// tool calls actually reach callPermission's allow paths, where steerContext is
// injected), never in "auto" (bypass) or "read-only" (plan mode — see the
// read-only sub-test); codex-cli NEVER (it has no permission-prompt-tool boundary
// in any mode — codex exec rejects every approval request outright, see
// codexMCPSpec's doc comment).
func TestSteerableForTurn(t *testing.T) {
	cases := []struct {
		provider, mode string
		want           bool
	}{
		{"claude-cli", "auto", false},
		{"claude-cli", "", false}, // "" resolves to auto/bypass
		{"claude-cli", "ask", true},
		{"claude-cli", "read-only", false},
		{"anthropic", "auto", true},
		{"minimax", "", true},
		{"codex-cli", "auto", false},
		{"codex-cli", "ask", false},
		{"codex-cli", "read-only", false},
	}
	for _, c := range cases {
		if got := steerableForTurn(c.provider, c.mode); got != c.want {
			t.Errorf("steerableForTurn(%q,%q) = %v, want %v", c.provider, c.mode, got, c.want)
		}
	}
}

// TestSteerableForTurnReadOnlyCLI pins the read-only case on its own, because it is
// the one that looks steerable but is not. read-only DOES wire the
// permission-prompt tool (climcp.PromptToolForMode), which is why this returned
// true and the backend told the user "steered". But read-only also runs the CLI
// under --permission-mode plan, where the CLI blocks mutations itself and every
// bridged tool sits on --allowedTools, so the only call reaching the prompt is
// ExitPlanMode — and callPermission hands that to callExitPlan before it can ever
// call steerContext. No boundary, no delivery: the honest answer is false, so the
// caller takes its "unsupported" path (queue + hint) instead of claiming the
// message landed. "ask" must stay true in the same breath — that is the mode whose
// write/exec tools do reach callPermission's allow paths.
func TestSteerableForTurnReadOnlyCLI(t *testing.T) {
	if steerableForTurn("claude-cli", "read-only") {
		t.Error(`steerableForTurn("claude-cli","read-only") = true, want false: ` +
			"plan mode reaches the prompt only via ExitPlanMode, which never delivers a steer")
	}
	if !steerableForTurn("claude-cli", "ask") {
		t.Error(`steerableForTurn("claude-cli","ask") = false, want true: ` +
			"the read-only fix must not disable the one mode that does deliver")
	}
}

// TestChatRunSteerStash verifies the claude-cli steer stash: setSteer holds the
// latest message and takeSteer consumes it exactly once (empty afterwards).
func TestChatRunSteerStash(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rs", "s-rs", "", func() {})
	defer runs.unregister("rs")

	if got := run.takeSteer(); got != "" {
		t.Fatalf("fresh run should have no steer, got %q", got)
	}
	run.setSteer("first")
	run.setSteer("second") // latest wins
	if got := run.takeSteer(); got != "second" {
		t.Fatalf("takeSteer = %q, want %q", got, "second")
	}
	if got := run.takeSteer(); got != "" {
		t.Fatalf("takeSteer should be empty after consume, got %q", got)
	}
}

// TestCallPermissionInjectsSteer verifies a pending steer is delivered as
// additionalContext on the auto-allow (RiskRead) tool boundary, and consumed.
func TestCallPermissionInjectsSteer(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rsi", "s-rsi", "", func() {})
	defer runs.unregister("rsi")
	b := &interactionBackend{runs: runs}

	run.setSteer("switch to the login bug instead")
	res, err := b.callPermission(context.Background(), run,
		json.RawMessage(`{"tool_name":"Read","input":{"file_path":"x.go"}}`))
	if err != nil {
		t.Fatalf("callPermission: %v", err)
	}
	var decision struct {
		Behavior          string `json:"behavior"`
		AdditionalContext string `json:"additionalContext"`
	}
	if uErr := json.Unmarshal([]byte(res.Text), &decision); uErr != nil {
		t.Fatalf("decision not JSON: %v (%s)", uErr, res.Text)
	}
	if decision.Behavior != "allow" {
		t.Fatalf("RiskRead should auto-allow, got %q", decision.Behavior)
	}
	if !strings.Contains(decision.AdditionalContext, "switch to the login bug instead") {
		t.Fatalf("steer not injected as additionalContext: %q", decision.AdditionalContext)
	}
	if got := run.takeSteer(); got != "" {
		t.Fatalf("steer should be consumed after delivery, still have %q", got)
	}
}

// TestCallPermissionNoSteerIsPlain verifies the decision stays byte-identical to
// the pre-steer contract when nothing is stashed (no additionalContext key).
func TestCallPermissionNoSteerIsPlain(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rsp", "s-rsp", "", func() {})
	defer runs.unregister("rsp")
	b := &interactionBackend{runs: runs}

	res, err := b.callPermission(context.Background(), run,
		json.RawMessage(`{"tool_name":"Read","input":{"file_path":"x.go"}}`))
	if err != nil {
		t.Fatalf("callPermission: %v", err)
	}
	if strings.Contains(res.Text, "additionalContext") {
		t.Fatalf("no steer pending, decision must not carry additionalContext: %s", res.Text)
	}
}

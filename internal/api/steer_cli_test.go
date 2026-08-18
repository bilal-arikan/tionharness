package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestSteerableForTurn pins the rule that gates the "unsupported" steer response:
// native providers steer in any mode; claude-cli only in "ask"/"read-only" (a
// permission-prompt boundary exists there), never in "auto" (bypass); codex-cli
// NEVER (it has no permission-prompt-tool boundary in any mode — codex exec
// rejects every approval request outright, see codexMCPSpec's doc comment).
func TestSteerableForTurn(t *testing.T) {
	cases := []struct {
		provider, mode string
		want           bool
	}{
		{"claude-cli", "auto", false},
		{"claude-cli", "", false}, // "" resolves to auto/bypass
		{"claude-cli", "ask", true},
		{"claude-cli", "read-only", true},
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

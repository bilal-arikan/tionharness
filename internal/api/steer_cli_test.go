package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// CLI steering requires the Interaction MCP endpoint, independent of permission mode.
func TestSteerableForTurn(t *testing.T) {
	for _, provider := range []string{"claude-cli", "codex-cli", "anthropic", "minimax"} {
		for _, bridge := range []bool{false, true} {
			want := bridge || (provider != "claude-cli" && provider != "codex-cli")
			if got := steerableForTurn(provider, bridge); got != want {
				t.Errorf("provider=%s bridge=%v: got %v, want %v", provider, bridge, got, want)
			}
		}
	}
}

// TestCallPermissionInjectsSteer verifies a pending steer is delivered as
// additionalContext on the auto-allow (RiskRead) tool boundary, and consumed.
func TestCallPermissionInjectsSteer(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("rsi", "s-rsi", "", func() {})
	defer runs.unregister("rsi")
	b := &interactionBackend{runs: runs}

	run.setProvider("claude-cli")
	run.setSteerable(true)
	deliverSteer(run, "switch to the login bug instead")
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
	if got := run.takeCLISteer(); len(got) != 0 {
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

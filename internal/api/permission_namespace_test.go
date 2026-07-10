package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/agent"
)

// TestPermissionPromptStripsNamespace closes Doc 52 YENI-A: when claude-cli calls the
// permission prompt for a gateway-activated extended tool, it passes the NAMESPACED name
// (mcp__tionswarm_extended__<tool>). The handler must strip the namespace so the tool is
// classified by its real bare risk — a read-only tool then auto-allows instead of
// needlessly prompting (and standing grants match). Uses get_session_info (RiskRead), so
// the auto-allow path returns without touching the grant store or emitting a step.
func TestPermissionPromptStripsNamespace(t *testing.T) {
	runs := newChatRuns()
	run := runs.register("r1", "s1", "", func() {})
	b := &interactionBackend{runs: runs, tun: agent.NewTunables()}

	args := json.RawMessage(`{"tool_name":"mcp__tionswarm_extended__get_session_info","input":{}}`)
	res, err := b.callPermission(context.Background(), run, args)
	if err != nil {
		t.Fatalf("callPermission err: %v", err)
	}
	if !strings.Contains(res.Text, `"behavior":"allow"`) {
		t.Fatalf("namespaced read-risk tool must auto-allow via bare classification, got %q", res.Text)
	}
}

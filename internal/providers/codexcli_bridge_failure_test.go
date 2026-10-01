package providers

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCodexRequiredBridgeFailureIsVisibleAndNeverDropped(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_TEST_REQUIRED_BRIDGE", "1")
	for _, preflight := range []bool{false, true} {
		t.Run(fmt.Sprintf("preflight=%v", preflight), func(t *testing.T) {
			server := CLIMCPServer{Command: "required-bridge"}
			if preflight {
				server = CLIMCPServer{Transport: "http", URL: "http://127.0.0.1:1/mcp"}
			}
			c := &CodexCLI{binPath: self, configDir: t.TempDir(), mcpServers: map[string]CLIMCPServer{"tionharness_interaction": server}, mcpProbe: func(context.Context, string) bool { return !preflight }}
			var live []TraceStep
			resp, err := c.completeWithArgs(context.Background(), []string{"-test.run=TestCodexHelperRequiredBridgeFailure", "-test.v=false"}, "prompt", "gpt-test", Request{OnEvent: func(st TraceStep) { live = append(live, st) }})
			if err == nil || resp != nil {
				t.Fatalf("required bridge failure ran without tools: %+v %v", resp, err)
			}
			trace := CLIErrorTrace(err)
			if len(trace) == 0 || trace[len(trace)-1].Status != "failed" || trace[len(trace)-1].Operation != "mcp_startup" {
				t.Fatalf("failure diagnostic missing: %+v %v", trace, err)
			}
			for _, st := range append(trace, live...) {
				if st.Status == "degraded" {
					t.Fatalf("required bridge was disabled: %+v", st)
				}
			}
			if !preflight && (!strings.Contains(trace[0].Output, "handshake failed") || strings.Contains(trace[0].Output, "private-key")) {
				t.Fatalf("bounded redacted first error lost: %+v", trace)
			}
		})
	}
}

func TestCodexHelperRequiredBridgeFailure(t *testing.T) {
	if os.Getenv("CODEX_TEST_REQUIRED_BRIDGE") == "" {
		t.Skip("subprocess fixture only")
	}
	fmt.Fprintln(os.Stderr, "MCP client for `tionharness_interaction` failed to start: handshake failed; token=private-key")
	os.Exit(1)
}

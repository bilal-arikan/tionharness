package providers

import (
	"strings"
	"testing"
)

func TestCodexToolPolicyRenderingAndReset(t *testing.T) {
	c := NewCodexCLI("codex", "", "")
	c.ConfigureCLIMCP(CLIMCPSpec{DisableNativeShell: true, Servers: map[string]CLIMCPServer{
		"probe": {Command: "probe", EnabledTools: []string{"read", "quote\"tool"}, DisabledTools: []string{"write"}},
		"empty": {Command: "probe", EnabledTools: []string{}},
	}})
	text := renderCodexConfig(c.buildConfig(Request{}))
	for _, want := range []string{"shell_tool = false", `enabled_tools = ["quote\"tool", "read"]`, `disabled_tools = ["write"]`, "enabled_tools = []"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Count(text, "[features]") != 1 {
		t.Fatal("duplicate feature table")
	}
	c.ConfigureCLIMCP(CLIMCPSpec{})
	text = renderCodexConfig(c.buildConfig(Request{}))
	if strings.Contains(text, "shell_tool") || strings.Contains(text, "mcp_servers") {
		t.Fatalf("stale policy survived reset: %s", text)
	}
}

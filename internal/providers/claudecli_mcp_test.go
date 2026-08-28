package providers

import "testing"

func TestClaudeCLIMCPArgsDisallowedToolsWithoutConfig(t *testing.T) {
	c := &ClaudeCLI{disallowedTools: []string{"WebSearch", "WebFetch"}}
	got := c.mcpArgs()

	if containsString(got, "--mcp-config") {
		t.Fatalf("mcpArgs() = %v, must omit --mcp-config without a config path", got)
	}
	if !containsString(got, "--disallowedTools") {
		t.Fatalf("mcpArgs() = %v, must include --disallowedTools", got)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

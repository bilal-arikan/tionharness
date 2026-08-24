package agent

import "testing"

func TestCatalogDisplayName(t *testing.T) {
	tests := []struct {
		name string
		cli  bool
		want string
	}{
		{"lazy built-in", true, "mcp__tionharness_extended__activate_tools"},
		{"interaction MCP idempotent", true, "mcp__tionharness_interaction__activate_tools"},
		{"extended MCP idempotent", true, "mcp__tionharness_extended__notify"},
		{"external MCP", true, "mcp__codebase-memory-mcp__search_code"},
		{"native", false, "Read"},
	}
	inputs := []string{
		"activate_tools",
		"mcp__tionharness_interaction__activate_tools",
		"mcp__tionharness_extended__notify",
		"codebase-memory-mcp__search_code",
		"Read",
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := catalogDisplayName(inputs[i], tt.cli)
			if !ok {
				t.Fatal("catalogDisplayName returned ok=false")
			}
			if got != tt.want {
				t.Fatalf("catalogDisplayName(%q, %v) = %q, want %q", inputs[i], tt.cli, got, tt.want)
			}
		})
	}
}

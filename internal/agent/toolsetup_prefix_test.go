package agent

import "testing"

func TestCatalogDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     string
	}{
		{"lazy built-in", "claude-cli", "mcp__tionharness_extended__activate_tools"},
		{"interaction MCP idempotent", "claude-cli", "mcp__tionharness_interaction__activate_tools"},
		{"extended MCP idempotent", "claude-cli", "mcp__tionharness_extended__notify"},
		{"external MCP", "claude-cli", "mcp__codebase-memory-mcp__search_code"},
		{"native", "anthropic", "Read"},
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
			got, ok := catalogDisplayName(inputs[i], tt.provider)
			if !ok {
				t.Fatal("catalogDisplayName returned ok=false")
			}
			if got != tt.want {
				t.Fatalf("catalogDisplayName(%q, %q) = %q, want %q", inputs[i], tt.provider, got, tt.want)
			}
		})
	}
}

// TestCatalogDisplayNameWebToolsAreWithheldFromCLIs pins that no CLI dialect sees
// TionHarness's WebFetch/WebSearch: every CLI ships its own web tools (codex-cli's
// web_search searches and opens pages). The other exclusions (run_subagent,
// run_code, deactivate_tools) are about dispatch context; the native path keeps
// the bare names.
func TestCatalogDisplayNameWebToolsAreWithheldFromCLIs(t *testing.T) {
	tests := []struct {
		tool     string
		provider string
		want     string // "" means the tool must be dropped (ok=false)
	}{
		{"WebFetch", "claude-cli", ""},
		{"WebSearch", "claude-cli", ""},
		{"WebFetch", "", ""}, // empty provider = keyless claude-cli default
		{"WebSearch", "", ""},
		{"WebFetch", "codex-cli", ""},
		{"WebSearch", "codex-cli", ""},
		{"WebFetch", "opencode-cli", ""},
		{"run_subagent", "claude-cli", ""},
		{"run_subagent", "codex-cli", ""},
		{"run_code", "codex-cli", ""},
		{"deactivate_tools", "codex-cli", ""},
		{"WebFetch", "anthropic", "WebFetch"}, // native path is unaffected
	}
	for _, tt := range tests {
		got, ok := catalogDisplayName(tt.tool, tt.provider)
		if tt.want == "" {
			if ok {
				t.Errorf("catalogDisplayName(%q, %q) = %q, want dropped", tt.tool, tt.provider, got)
			}
			continue
		}
		if !ok || got != tt.want {
			t.Errorf("catalogDisplayName(%q, %q) = (%q, %v), want (%q, true)", tt.tool, tt.provider, got, ok, tt.want)
		}
	}
}

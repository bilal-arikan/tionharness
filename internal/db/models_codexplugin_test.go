package db

import "testing"

func TestCodexMarketplaceNameReserved(t *testing.T) {
	// Codex refuses to register these from any user source; rendering one would
	// fail every turn under --strict-config.
	for _, name := range []string{"openai-bundled", "OpenAI-Bundled", " openai-curated-remote "} {
		if !CodexMarketplaceNameReserved(name) {
			t.Errorf("%q must read as reserved", name)
		}
	}
	if CodexMarketplaceNameReserved("local-openai") {
		t.Error("a user-chosen name must not read as reserved")
	}
}

func TestValidCodexMarketplaceName(t *testing.T) {
	for _, name := range []string{"local-openai", "team_mp", "mp1"} {
		if !ValidCodexMarketplaceName(name) {
			t.Errorf("%q should be valid", name)
		}
	}
	for _, name := range []string{"", "   ", "openai-bundled", "has space", "dot.name", "at@name"} {
		if ValidCodexMarketplaceName(name) {
			t.Errorf("%q should be rejected", name)
		}
	}
}

func TestSplitCodexPluginSelector(t *testing.T) {
	plugin, marketplace, ok := SplitCodexPluginSelector("visualize@local-openai")
	if !ok || plugin != "visualize" || marketplace != "local-openai" {
		t.Fatalf("got %q/%q ok=%v", plugin, marketplace, ok)
	}
	for _, bad := range []string{"", "visualize", "@mp", "visualize@", "  "} {
		if _, _, ok := SplitCodexPluginSelector(bad); ok {
			t.Errorf("%q should not parse as a selector", bad)
		}
	}
}

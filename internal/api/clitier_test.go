package api

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// TestCLITierProjectsVisibility verifies the 4-tier visibility model projects onto
// the claude-cli two-state wire as documented: full→core, summary/name-only→
// extended, hidden→neither; and the behavioral core set stays eager regardless of
// visibility. This is the fix for the CLI path ignoring per-tool visibility.
func TestCLITierProjectsVisibility(t *testing.T) {
	vis := map[string]string{
		"update_session": tools.VisibilityFull,     // extended-by-default, promoted
		"notify":         tools.VisibilitySummary,  // deferred
		"focus_view":     tools.VisibilityNameOnly, // deferred
		"schedule_wake":  tools.VisibilityHidden,   // dropped
		"todo_write":     tools.VisibilityHidden,   // required-core: stays eager
	}
	visOf := func(name string) string {
		if v, ok := vis[name]; ok {
			return v
		}
		return tools.VisibilityFull
	}

	cases := []struct{ name, want string }{
		{"permission_prompt", "core"}, // required core (behavioral), always eager
		{"todo_write", "core"},        // required core wins over hidden visibility
		{"update_session", "core"},    // full → eager
		{"notify", "extended"},        // summary → deferred
		{"focus_view", "extended"},    // name-only → deferred
		{"schedule_wake", "hidden"},   // hidden → advertised on neither tier
		{"create_agent", "core"},      // full (default in stub) → eager
	}
	for _, c := range cases {
		if got := cliTier(c.name, visOf); got != c.want {
			t.Errorf("cliTier(%q) = %q, want %q", c.name, got, c.want)
		}
	}

	// nil visOf reproduces the historical static split: core set eager, everything
	// else deferred, nothing hidden.
	if cliTier("update_session", nil) != "extended" {
		t.Error("nil visOf must defer non-core tools (static split)")
	}
	if cliTier("ask_user", nil) != "core" {
		t.Error("nil visOf must keep the core set eager")
	}
}

// TestSplitInteractionTiersHonorsVisibility verifies splitInteractionTiers routes a
// full-promoted tool to core, a summary/name-only tool to extended, and drops a
// hidden tool from BOTH wire tiers.
func TestSplitInteractionTiersHonorsVisibility(t *testing.T) {
	visOf := func(name string) string {
		switch name {
		case "update_session":
			return tools.VisibilityFull
		case "schedule_wake":
			return tools.VisibilityHidden
		default:
			return tools.VisibilityNameOnly
		}
	}
	static := []string{"ask_user", "update_session", "notify", "schedule_wake"}
	bridge := []providers.ToolDef{{Name: "create_agent"}}
	core, ext := splitInteractionTiers(static, bridge, visOf)

	if !contains(core, "ask_user") {
		t.Errorf("behavioral ask_user must stay core: %v", core)
	}
	if !contains(core, "update_session") {
		t.Errorf("full-promoted update_session must be core: %v", core)
	}
	if !contains(ext, "notify") || !contains(ext, "create_agent") {
		t.Errorf("name-only tools must be extended: %v", ext)
	}
	if contains(core, "schedule_wake") || contains(ext, "schedule_wake") {
		t.Errorf("hidden schedule_wake must appear on neither tier: core=%v ext=%v", core, ext)
	}
}

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

// TestCoordinationToolsAreEager pins FND-8ea05c42: the coordinator's worker-driving
// tools must ride EAGER (core, alwaysLoad) on the CLI wire, never deferred behind
// ToolSearch — a coordinator that has to discover spawn_worker before calling it is
// exactly the drift that ends in a phantom-spawn stall. They are registered as plain
// builtins with the default Full visibility (nothing demotes them to lazy/name-only/
// hidden), so the per-agent resolver reports Full and cliTier routes them to core.
func TestCoordinationToolsAreEager(t *testing.T) {
	// The live path uses the registry's VisibilityOf, which returns Full for any tool
	// not explicitly demoted — coordination tools are never demoted. Model that here.
	visOf := func(string) string { return tools.VisibilityFull }
	for _, name := range []string{"spawn_worker", "list_workers", "send_to_worker", "stop_worker"} {
		if got := cliTier(name, visOf); got != "core" {
			t.Errorf("cliTier(%q) = %q, want core (coordination tools must stay eager)", name, got)
		}
	}
	// And they must actually reach the core wire tier when bridged, not just classify.
	bridge := []providers.ToolDef{{Name: "spawn_worker"}, {Name: "list_workers"}}
	core, ext := splitInteractionTiers(nil, bridge, visOf)
	for _, name := range []string{"spawn_worker", "list_workers"} {
		if !contains(core, name) {
			t.Errorf("%q must be advertised on the eager core tier: core=%v ext=%v", name, core, ext)
		}
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

package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// plannerAgent is the profile shape from defaultSubagentProfiles["planner"]: a
// read-only allowlist that names no skill tool, while its system prompt still
// renders "# Available Skills" and tells it to load a matching skill.
func plannerAgent(t *testing.T) db.Agent {
	t.Helper()
	a := profileAgent(t, "Read", "LS", "Glob", "Grep", "WebFetch")
	a.Name = "Worker: Planner"
	a.SystemKey = "subagent-planner"
	return a
}

// TestToolFilterExemptsSkillToolsFromAllowlist is the regression for a live
// planner session (SES525) that never called use_skill: the profile allowlist
// stripped the skill tools out of the catalog entirely, so the prompt advertised
// skills the worker had no way to load.
func TestToolFilterExemptsSkillToolsFromAllowlist(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	filter := rt.toolFilter(context.Background(), plannerAgent(t))
	if filter == nil {
		t.Fatal("an agent with an allowlist must produce a filter")
	}

	for _, name := range tools.SkillToolNames {
		if !filter(name) {
			t.Errorf("%s must survive a profile allowlist — the prompt tells every agent to use it", name)
		}
		// CLI providers see the interaction-bridge form of the same tool.
		bridged := "mcp__tionharness_interaction__" + name
		if !filter(bridged) {
			t.Errorf("%s must survive a profile allowlist too", bridged)
		}
	}
	// The allowlist must still restrict WORK tools, or the exemption has quietly
	// become "profiles are unrestricted".
	for _, name := range []string{"Write", "Edit", "Bash"} {
		if filter(name) {
			t.Errorf("%s is not on the planner allowlist and must stay unavailable", name)
		}
	}
}

// TestToolFilterSkillToolsStillBlockable proves the exemption is scoped to the
// ALLOWLIST: an explicit per-agent denylist is a deliberate decision about this
// agent and must still win.
func TestToolFilterSkillToolsStillBlockable(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	a := plannerAgent(t)
	a.BlockedTools = `["use_skill"]`
	filter := rt.toolFilter(context.Background(), a)
	if filter == nil {
		t.Fatal("expected a filter")
	}
	if filter("use_skill") {
		t.Error("an explicitly blocked skill tool must stay blocked")
	}
	if !filter("skill_search") {
		t.Error("blocking one skill tool must not blanket-block the surface")
	}
}

// TestToolFilterSkillToolsWorkspaceDisabled proves the workspace-level switch
// still wins over the exemption.
func TestToolFilterSkillToolsWorkspaceDisabled(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	if err := rt.db.SetWorkspaceToolConfig(ctx, db.WorkspaceToolConfig{DisabledTools: []string{"use_skill"}}); err != nil {
		t.Fatalf("disable use_skill: %v", err)
	}
	filter := rt.toolFilter(ctx, plannerAgent(t))
	if filter == nil {
		t.Fatal("expected a filter")
	}
	if filter("use_skill") {
		t.Error("a workspace-disabled skill tool must stay disabled")
	}
	if !filter("skill_search") {
		t.Error("disabling one skill tool must not blanket-disable the surface")
	}
}

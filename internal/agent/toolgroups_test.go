package agent

import (
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// A "group:" override at the blocked tier bans every built-in of that category,
// and nothing else — MCP tools group by server, not by category.
func TestBlockFuncGroupKeyBlocksWholeCategory(t *testing.T) {
	blocked := blockFunc(db.Agent{ToolOverrides: `{"group:files":"blocked"}`})
	if blocked == nil {
		t.Fatal("group key must produce a predicate")
	}
	for _, name := range []string{"Read", "Write", "Bash"} {
		if !blocked(name) {
			t.Fatalf("%s should be blocked by group:files", name)
		}
	}
	for _, name := range []string{"WebSearch", "mcp__linear__issue"} {
		if blocked(name) {
			t.Fatalf("%s must not be blocked by group:files", name)
		}
	}
}

// An EXACT override beats the group it falls into: banning group:files while
// pinning Read to a visibility tier keeps Read usable.
func TestBlockFuncExactNameBeatsGroup(t *testing.T) {
	blocked := blockFunc(db.Agent{ToolOverrides: `{"group:files":"blocked","Read":"summary"}`})
	if blocked("Read") {
		t.Fatal("explicit non-blocked override must beat the group ban")
	}
	if !blocked("Write") {
		t.Fatal("the rest of the group stays blocked")
	}
}

// Same specificity rule on the visibility side: the group is expanded across the
// catalog first, then exact names overwrite it.
func TestApplyVisibilityOverridesGroupExpansionAndPrecedence(t *testing.T) {
	reg := tools.NewRegistry()
	names := []string{"Read", "Write", "Bash", "WebSearch", "mcp__linear__issue"}
	applyVisibilityOverrides(reg, map[string]string{
		"group:files": tools.VisibilityHidden,
		"Read":        tools.VisibilityFull,
	}, names)

	for _, n := range []string{"Write", "Bash"} {
		if reg.VisibilityOf(n) != tools.VisibilityHidden {
			t.Fatalf("group not applied to %s: %q", n, reg.VisibilityOf(n))
		}
	}
	if reg.VisibilityOf("Read") != tools.VisibilityFull {
		t.Fatalf("exact name must beat the group, got %q", reg.VisibilityOf("Read"))
	}
	if reg.VisibilityOf("mcp__linear__issue") != tools.VisibilityFull {
		t.Fatalf("MCP tool must be untouched by a category group, got %q", reg.VisibilityOf("mcp__linear__issue"))
	}
}

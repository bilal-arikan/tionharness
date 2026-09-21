package repair

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// zvecSearchTool is built through the registry's own namespace helper, for the
// reason searchTool is (repair_test.go).
var zvecSearchTool = mcp.NamespaceTool("zvec_grep", "zvec_grep_search")

// zvecSearchSchema mirrors zvec_grep_search as served by zvec-grep 0.2.2, trimmed
// to what the guard reads: `root` is the only required argument.
const zvecSearchSchema = `{"type":"object","properties":{"root":{"type":"string"},"query":{"type":"string"}},"required":["root"]}`

// zvecIndexMissing is zvec-grep 0.2.2's verbatim error for a root without an index.
func zvecIndexMissing(root string) providers.ToolResult {
	return errResult("[INDEX_MISSING] Indexed search requires a built zvec-grep index for " + root +
		". Use an available exact-search fallback when it is sufficient. Creating or rebuilding a persistent index requires explicit user authorization.")
}

func TestIsZvecGrepTool(t *testing.T) {
	for name, want := range map[string]bool{
		zvecSearchTool:                     true,
		"mcp__zvec_grep__zvec_grep_search": true,
		"semantic__zvec_grep_rg":           true,
		searchTool:                         false,
		"zvec_grep_search":                 false, // not namespaced: not an MCP call
		"zvec_grep__search_code":           false,
	} {
		if got := IsZvecGrepTool(name); got != want {
			t.Errorf("IsZvecGrepTool(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestPrefillArgs_FillsZvecGrepRootFromSessionCwd(t *testing.T) {
	cwd := t.TempDir()
	call := mcpCall(zvecSearchTool, map[string]any{"query": "how are results ranked"})
	missing := MissingRequiredArgs(json.RawMessage(zvecSearchSchema), call.Input)
	fixed, ok := PrefillArgs(call, missing, cwd)
	if !ok {
		t.Fatal("expected root to be filled from the session cwd")
	}
	if got := callStringArg(fixed, "root"); got != filepath.Clean(cwd) {
		t.Errorf("root = %q, want %q", got, cwd)
	}
	if got := callStringArg(fixed, "query"); got != "how are results ranked" {
		t.Errorf("prefill dropped another argument; query = %q", got)
	}
	if got := MissingRequiredArgs(json.RawMessage(zvecSearchSchema), fixed.Input); got != nil {
		t.Errorf("prefilled call still reports missing args: %v", got)
	}
}

func TestPrefillArgs_RootOnlyForZvecGrepAndAbsoluteCwd(t *testing.T) {
	// Another server's `root` means whatever that server says it means.
	other := mcpCall(mcp.NamespaceTool("filesystem", "list_directory"), map[string]any{})
	if _, ok := PrefillArgs(other, []string{"root"}, t.TempDir()); ok {
		t.Error("root was prefilled for a non-zvec-grep tool")
	}
	call := mcpCall(zvecSearchTool, map[string]any{})
	for _, cwd := range []string{"", "   ", filepath.Join("relative", "dir")} {
		if _, ok := PrefillArgs(call, []string{"root"}, cwd); ok {
			t.Errorf("root prefilled from the non-absolute cwd %q", cwd)
		}
	}
}

func TestRepairZvecGrep_IndexMissingPoisonsAndPlansSessionIndex(t *testing.T) {
	cwd := t.TempDir()
	m := NewGuard()
	call := mcpCall(zvecSearchTool, map[string]any{"root": filepath.Join(cwd, "internal"), "query": "x"})
	plan, ok := m.RepairZvecGrep(call, zvecIndexMissing(cwd), cwd)
	if !ok {
		t.Fatal("an [INDEX_MISSING] result must be repaired")
	}
	if plan.IndexPath != filepath.Clean(cwd) {
		t.Errorf("IndexPath = %q, want the session cwd", plan.IndexPath)
	}
	if plan.Fixed != nil {
		t.Error("zvec-grep has no argument to correct; no retry expected")
	}
	for _, want := range []string{"[mcp repair]", "Glob/Grep", "zg index"} {
		if !strings.Contains(plan.Hint, want) {
			t.Errorf("hint missing %q: %s", want, plan.Hint)
		}
	}
	blocked, msg := m.Precheck(call)
	if !blocked {
		t.Fatal("an identical repeat must be refused this turn")
	}
	if strings.Contains(msg, "list_projects") {
		t.Errorf("a zvec-grep repeat got codebase-memory guidance: %s", msg)
	}
}

func TestRepairZvecGrep_ForeignRootIsNotIndexed(t *testing.T) {
	session, elsewhere := t.TempDir(), t.TempDir()
	call := mcpCall(zvecSearchTool, map[string]any{"root": elsewhere, "query": "x"})
	plan, ok := NewGuard().RepairZvecGrep(call, zvecIndexMissing(elsewhere), session)
	if !ok {
		t.Fatal("expected a repair")
	}
	if plan.IndexPath != "" {
		t.Errorf("a root outside the session repository triggered an index of %q", plan.IndexPath)
	}
}

func TestRepairZvecGrep_IgnoresEverythingElse(t *testing.T) {
	cwd := t.TempDir()
	m := NewGuard()
	call := mcpCall(zvecSearchTool, map[string]any{"root": cwd, "query": "x"})
	if _, ok := m.RepairZvecGrep(call, providers.ToolResult{Content: "freshness: fresh"}, cwd); ok {
		t.Error("a successful result was repaired")
	}
	if _, ok := m.RepairZvecGrep(call, errResult("Input validation error: root: expected string"), cwd); ok {
		t.Error("an unrelated error was repaired")
	}
	cbm := mcpCall(searchTool, map[string]any{"project": "P"})
	if _, ok := m.RepairZvecGrep(cbm, zvecIndexMissing(cwd), cwd); ok {
		t.Error("a codebase-memory call was handled by the zvec-grep repair")
	}
	// And the codebase-memory repair leaves zvec-grep's marker alone.
	if _, ok := m.Repair(call, zvecIndexMissing(cwd), cwd); ok {
		t.Error("the codebase-memory repair claimed an [INDEX_MISSING] result")
	}
}

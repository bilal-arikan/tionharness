package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/mcp"
)

func mcpEntry(server, tool string) mcp.CatalogEntry {
	return mcp.CatalogEntry{
		Server:         server,
		NamespacedName: mcp.NamespaceTool(server, tool),
		Tool:           mcp.Tool{Name: tool, Description: tool + " description"},
	}
}

func noopCaller(context.Context, string, json.RawMessage) (mcp.CallToolResult, error) {
	return mcp.CallToolResult{}, nil
}

// AppendMCP grows the catalog without disturbing what is already attached, and
// stamps the merged names with the same lazy + name-only marks AttachMCP applies.
func TestAppendMCPMergesWithSameMarks(t *testing.T) {
	r := NewRegistry()
	r.AttachMCP(
		[]mcp.CatalogEntry{mcpEntry("alpha", "one")},
		map[string]mcp.ServerConfig{"alpha": {Name: "alpha"}},
		noopCaller,
	)
	r.AppendMCP(
		[]mcp.CatalogEntry{mcpEntry("beta", "two")},
		map[string]mcp.ServerConfig{"beta": {Name: "beta"}},
	)

	names := map[string]bool{}
	for _, d := range r.Defs(nil) {
		names[d.Name] = true
	}
	if !names["alpha__one"] {
		t.Errorf("the originally attached tool was lost by AppendMCP")
	}
	if !names["beta__two"] {
		t.Errorf("the appended tool is missing from the catalog")
	}
	// Same tier treatment as AttachMCP: lazy, and name-only via the mcp:* row.
	if !r.IsLazy("beta__two") {
		t.Errorf("beta__two is not lazy; AppendMCP must mirror AttachMCP's marks")
	}
	if got, want := r.VisibilityOf("beta__two"), r.VisibilityOf("alpha__one"); got != want {
		t.Errorf("appended tier = %q, attached tier = %q; they must match", got, want)
	}
}

// A re-listed server may have changed a tool's schema: appending the same
// namespaced name replaces the entry rather than duplicating it.
func TestAppendMCPReplacesExistingName(t *testing.T) {
	r := NewRegistry()
	r.AttachMCP(
		[]mcp.CatalogEntry{mcpEntry("alpha", "one")},
		map[string]mcp.ServerConfig{"alpha": {Name: "alpha"}},
		noopCaller,
	)
	updated := mcpEntry("alpha", "one")
	updated.Tool.Description = "new description"
	r.AppendMCP([]mcp.CatalogEntry{updated}, nil)

	count, desc := 0, ""
	for _, d := range r.Defs(nil) {
		if d.Name == "alpha__one" {
			count++
			desc = d.Description
		}
	}
	if count != 1 {
		t.Fatalf("alpha__one appears %d times, want 1 (append must replace, not duplicate)", count)
	}
	if desc != "new description" {
		t.Errorf("description = %q, want the re-listed one", desc)
	}
}

// Appending before AttachMCP would register tools with no dispatcher behind
// them — silently unreachable. That is a programming error, so it panics rather
// than half-working.
func TestAppendMCPBeforeAttachPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatalf("AppendMCP without a caller must panic, not register unreachable tools")
		}
	}()
	r := NewRegistry()
	r.AppendMCP([]mcp.CatalogEntry{mcpEntry("alpha", "one")}, nil)
}

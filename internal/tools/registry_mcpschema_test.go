package tools

import (
	"encoding/json"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

func TestMCPSchema(t *testing.T) {
	const schema = `{"type":"object","required":["project"]}`
	reg := NewRegistry()
	reg.AttachMCP([]mcp.CatalogEntry{{
		Server:         "codebase-memory-mcp",
		NamespacedName: "codebase-memory-mcp__search_code",
		Tool:           mcp.Tool{Name: "search_code", InputSchema: json.RawMessage(schema)},
	}}, nil, nil)

	if got := string(reg.MCPSchema("codebase-memory-mcp__search_code")); got != schema {
		t.Errorf("MCPSchema() = %q, want %q", got, schema)
	}
	// An unknown name and a built-in both resolve to "no declared contract", which
	// callers must read as "do not validate" rather than "no required fields".
	if reg.MCPSchema("codebase-memory-mcp__nope") != nil {
		t.Error("unknown MCP tool must yield a nil schema")
	}
	if reg.MCPSchema("Bash") != nil {
		t.Error("built-in tool must yield a nil schema")
	}
}

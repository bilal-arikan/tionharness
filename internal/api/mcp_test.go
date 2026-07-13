package api

import (
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// TestMCPSignatureDedup verifies the importable-server identity: same wiring (name
// + transport + command + args + url) yields the same signature regardless of id
// or enabled state, while any wiring difference yields a distinct one. This is what
// dedupes a server shared across workspaces and hides ones already present here.
func TestMCPSignatureDedup(t *testing.T) {
	base := db.MCPServer{
		ID: "MCP1", Name: "codebase-memory-mcp", Transport: "stdio",
		Command: "cbm.exe", Args: "[]", URL: "", Enabled: true,
	}
	// Same wiring, different id + disabled → same signature.
	twin := base
	twin.ID = "MCP99"
	twin.Enabled = false
	if mcpSignature(base) != mcpSignature(twin) {
		t.Fatalf("identical wiring must share a signature:\n%q\n%q", mcpSignature(base), mcpSignature(twin))
	}
	// Name compared case-insensitively (display casing must not fork the identity).
	upper := base
	upper.Name = "Codebase-Memory-MCP"
	if mcpSignature(base) != mcpSignature(upper) {
		t.Fatalf("name casing must not change the signature")
	}
	// Any wiring change → distinct signature.
	for _, diff := range []func(m *db.MCPServer){
		func(m *db.MCPServer) { m.Command = "other.exe" },
		func(m *db.MCPServer) { m.Args = `["-y"]` },
		func(m *db.MCPServer) { m.Transport = "http" },
		func(m *db.MCPServer) { m.URL = "https://x" },
		func(m *db.MCPServer) { m.Name = "different" },
	} {
		m := base
		diff(&m)
		if mcpSignature(m) == mcpSignature(base) {
			t.Fatalf("a wiring change must produce a distinct signature, got equal for %+v", m)
		}
	}
}

package mcp

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestLiveFilesystem exercises the stdio client against the real
// @modelcontextprotocol/server-filesystem (launched via npx). It is gated on
// SWARMGO_MCP_LIVE=1 so it never runs in CI without Node/network.
//
//	SWARMGO_MCP_LIVE=1 SWARMGO_MCP_DIR=<dir> go test ./internal/mcp -run Live -v
func TestLiveFilesystem(t *testing.T) {
	if os.Getenv("SWARMGO_MCP_LIVE") != "1" {
		t.Skip("set SWARMGO_MCP_LIVE=1 to run the live MCP test")
	}
	dir := os.Getenv("SWARMGO_MCP_DIR")
	if dir == "" {
		t.Fatal("SWARMGO_MCP_DIR is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := ServerConfig{
		Name:      "filesystem",
		Transport: MCPTransportStdio,
		Command:   "npx",
		Args:      []string{"-y", "@modelcontextprotocol/server-filesystem", dir},
	}

	tools, err := ListServerTools(ctx, cfg)
	if err != nil {
		t.Fatalf("ListServerTools: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("expected at least one tool")
	}
	t.Logf("got %d tools: %v", len(tools), toolNames(tools))

	// Call list_directory on the allowed dir.
	args, _ := json.Marshal(map[string]string{"path": dir})
	cfgByServer := map[string]ServerConfig{"filesystem": cfg}
	res, err := CallNamespaced(ctx, cfgByServer, NamespaceTool("filesystem", "list_directory"), args)
	if err != nil {
		t.Fatalf("CallNamespaced: %v", err)
	}
	t.Logf("list_directory result: %s", res.Text)
	if res.IsError {
		t.Fatalf("tool reported error: %s", res.Text)
	}
}

func toolNames(tools []Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name
	}
	return out
}

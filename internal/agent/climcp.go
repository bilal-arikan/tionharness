package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/mcp"
)

// cliMCPConfig is the on-disk shape claude --mcp-config expects.
type cliMCPConfig struct {
	MCPServers map[string]cliMCPServer `json:"mcpServers"`
}

type cliMCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"` // sse | http
	URL     string            `json:"url,omitempty"`
}

// writeCLIMCPConfig renders every enabled MCP server into a temp config file
// for the claude CLI and returns the path, the allowlist of tool identifiers
// (one per server), and a cleanup func. Returns an empty path when there are no
// enabled servers.
func (r *Runtime) writeCLIMCPConfig(ctx context.Context) (string, []string, func(), error) {
	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		return "", nil, nil, err
	}
	if len(servers) == 0 {
		return "", nil, func() {}, nil
	}

	cfg := cliMCPConfig{MCPServers: map[string]cliMCPServer{}}
	var allowed []string
	for _, m := range servers {
		sc := toServerConfig(m)
		key, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(sc.Name, "x"))
		entry := cliMCPServer{}
		switch sc.Transport {
		case db.MCPTransportSSE, db.MCPTransportHTTP:
			entry.Type = sc.Transport
			entry.URL = sc.URL
		default:
			entry.Command = sc.Command
			entry.Args = sc.Args
			entry.Env = sc.Env
		}
		cfg.MCPServers[key] = entry
		allowed = append(allowed, "mcp__"+key)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, nil, err
	}
	f, err := os.CreateTemp("", "swarmgo-mcp-*.json")
	if err != nil {
		return "", nil, nil, err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, nil, err
	}
	_ = f.Close()

	cleanup := func() { _ = os.Remove(path) }
	r.logger.Info("cli mcp config written", "path", filepath.Base(path), "servers", len(servers))
	return path, allowed, cleanup, nil
}

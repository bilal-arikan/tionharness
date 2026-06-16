package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Transport kinds (mirrors db constants; duplicated to keep mcp dependency-free).
const (
	MCPTransportStdio = "stdio"
	MCPTransportSSE   = "sse"
	MCPTransportHTTP  = "http"
)

// ServerConfig is the transport-agnostic launch/connection spec for one MCP
// server. Callers build it from their own storage (e.g. db.MCPServer).
type ServerConfig struct {
	Name      string
	Transport string // stdio | sse | http
	Command   string
	Args      []string
	URL       string
	Env       map[string]string // extra environment variables
}

// envSlice renders Env as KEY=VALUE entries for exec.
func (c ServerConfig) envSlice() []string {
	out := make([]string, 0, len(c.Env))
	for k, v := range c.Env {
		out = append(out, k+"="+v)
	}
	return out
}

// dial opens a client for the configured transport. Only stdio is implemented;
// sse/http return a clear error so the UI can surface "not yet supported".
func (c ServerConfig) dial(ctx context.Context) (*StdioClient, error) {
	switch c.Transport {
	case MCPTransportStdio, "":
		if c.Command == "" {
			return nil, fmt.Errorf("mcp %q: stdio transport requires a command", c.Name)
		}
		return DialStdio(ctx, c.Command, c.Args, c.envSlice())
	case MCPTransportSSE, MCPTransportHTTP:
		return nil, fmt.Errorf("mcp %q: %s transport not yet supported", c.Name, c.Transport)
	default:
		return nil, fmt.Errorf("mcp %q: unknown transport %q", c.Name, c.Transport)
	}
}

// Namespaced tool naming: "<server>__<tool>" so tools from different servers
// never collide and the origin is recoverable.
const nsSep = "__"

// NamespaceTool builds the namespaced tool name.
func NamespaceTool(server, tool string) string {
	return sanitize(server) + nsSep + tool
}

// SplitNamespaced recovers (server, tool) from a namespaced name.
func SplitNamespaced(name string) (server, tool string, ok bool) {
	i := strings.Index(name, nsSep)
	if i < 0 {
		return "", "", false
	}
	return name[:i], name[i+len(nsSep):], true
}

// sanitize keeps tool names safe for provider tool schemas (letters/digits/_/-).
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// CatalogEntry is one tool in the aggregated catalog, tagged with its server.
type CatalogEntry struct {
	Server         string `json:"server"`
	NamespacedName string `json:"name"`
	Tool           Tool   `json:"tool"`
}

// ListServerTools dials a single server, lists its tools, and closes it. Used
// by the "test connection" endpoint and the catalog builder.
func ListServerTools(ctx context.Context, cfg ServerConfig) ([]Tool, error) {
	client, err := cfg.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	return client.ListTools(ctx)
}

// BuildCatalog connects to each server config and returns the union of their
// tools, namespaced. A server that fails to connect is skipped and its error
// recorded in errs (keyed by server name) rather than aborting the whole build.
func BuildCatalog(ctx context.Context, cfgs []ServerConfig) (entries []CatalogEntry, errs map[string]string) {
	errs = map[string]string{}
	for _, cfg := range cfgs {
		tools, err := ListServerTools(ctx, cfg)
		if err != nil {
			errs[cfg.Name] = err.Error()
			continue
		}
		for _, t := range tools {
			entries = append(entries, CatalogEntry{
				Server:         cfg.Name,
				NamespacedName: NamespaceTool(cfg.Name, t.Name),
				Tool:           t,
			})
		}
	}
	return entries, errs
}

// CallNamespaced dials the server owning the namespaced tool and invokes it.
// cfgByServer maps the sanitized server name to its config.
func CallNamespaced(ctx context.Context, cfgByServer map[string]ServerConfig, namespaced string, args json.RawMessage) (CallToolResult, error) {
	server, tool, ok := SplitNamespaced(namespaced)
	if !ok {
		return CallToolResult{}, fmt.Errorf("mcp: %q is not a namespaced tool name", namespaced)
	}
	cfg, ok := cfgByServer[server]
	if !ok {
		return CallToolResult{}, fmt.Errorf("mcp: no server %q for tool %q", server, namespaced)
	}
	client, err := cfg.dial(ctx)
	if err != nil {
		return CallToolResult{}, err
	}
	defer client.Close()
	return client.CallTool(ctx, tool, args)
}

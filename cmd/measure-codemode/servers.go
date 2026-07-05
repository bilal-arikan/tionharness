package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/bilal-arikan/tionswarm/internal/mcp"
)

// storedServer mirrors the on-disk db.MCPServer JSON (store/mcp-servers/*.json)
// closely enough to build an mcp.ServerConfig. Read-only: the harness never
// writes to a workspace store.
type storedServer struct {
	Name          string `json:"name"`
	Transport     string `json:"transport"`
	Command       string `json:"command"`
	Args          string `json:"args"`      // JSON-encoded []string
	EnvConfig     string `json:"envConfig"` // JSON-encoded map[string]string
	HeadersConfig string `json:"headersConfig"`
	URL           string `json:"url"`
	Enabled       bool   `json:"enabled"`
}

// loadServers scans every workspace under dataDir for enabled MCP server
// configs and returns them deduplicated by name (the same server configured in
// two workspaces is measured once).
func loadServers(dataDir string) ([]mcp.ServerConfig, error) {
	pattern := filepath.Join(dataDir, "workspaces", "*", "store", "mcp-servers", "*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	byName := map[string]mcp.ServerConfig{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f, err)
		}
		var s storedServer
		if err := json.Unmarshal(data, &s); err != nil {
			return nil, fmt.Errorf("parse %s: %w", f, err)
		}
		if !s.Enabled || s.Name == "" {
			continue
		}
		if _, seen := byName[s.Name]; seen {
			continue
		}
		var args []string
		_ = json.Unmarshal([]byte(s.Args), &args)
		env := map[string]string{}
		_ = json.Unmarshal([]byte(s.EnvConfig), &env)
		headers := map[string]string{}
		_ = json.Unmarshal([]byte(s.HeadersConfig), &headers)
		byName[s.Name] = mcp.ServerConfig{
			Name:      s.Name,
			Transport: s.Transport,
			Command:   s.Command,
			Args:      args,
			URL:       s.URL,
			Env:       env,
			Headers:   headers,
		}
	}
	out := make([]mcp.ServerConfig, 0, len(byName))
	for _, cfg := range byName {
		out = append(out, cfg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

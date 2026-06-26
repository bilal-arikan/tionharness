package db

import (
	"context"
	"sort"
)

func (d *DB) persistMCPLocked(m MCPServer) error {
	d.mcp[m.ID] = m
	return atomicWriteJSON(d.dir(dirMCP, m.ID+".json"), m)
}

// CreateMCPServer inserts a new MCP server config and returns the stored row.
func (d *DB) CreateMCPServer(ctx context.Context, m MCPServer) (MCPServer, error) {
	m.ID = d.nextID(idMCP)
	m.CreatedAt = now()
	if m.Transport == "" {
		m.Transport = MCPTransportStdio
	}
	if m.Args == "" {
		m.Args = "[]"
	}
	if m.EnvConfig == "" {
		m.EnvConfig = "{}"
	}
	if m.Scope == "" {
		m.Scope = "shared"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return m, d.persistMCPLocked(m)
}

// GetMCPServer loads an MCP server by id.
func (d *DB) GetMCPServer(ctx context.Context, id string) (MCPServer, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	m, ok := d.mcp[id]
	if !ok {
		return MCPServer{}, ErrNotFound
	}
	return m, nil
}

// ListMCPServers returns all configured servers, newest first.
func (d *DB) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := make([]MCPServer, 0, len(d.mcp))
	for _, m := range d.mcp {
		out = append(out, m)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// ListEnabledMCPServers returns only servers with Enabled = true.
func (d *DB) ListEnabledMCPServers(ctx context.Context) ([]MCPServer, error) {
	all, err := d.ListMCPServers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]MCPServer, 0, len(all))
	for _, m := range all {
		if m.Enabled {
			out = append(out, m)
		}
	}
	return out, nil
}

// SetMCPServerEnabled toggles a server on/off.
func (d *DB) SetMCPServerEnabled(ctx context.Context, id string, enabled bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	m, ok := d.mcp[id]
	if !ok {
		return ErrNotFound
	}
	m.Enabled = enabled
	return d.persistMCPLocked(m)
}

// DeleteMCPServer removes a server config.
func (d *DB) DeleteMCPServer(ctx context.Context, id string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.mcp[id]; !ok {
		return ErrNotFound
	}
	delete(d.mcp, id)
	return removeFile(d.dir(dirMCP, id+".json"))
}

// UpdateAgentTools sets an agent's tool access: whether MCP tools are offered
// and a per-agent denylist of tool-name patterns (JSON array). Agents reach all
// workspace-active tools by default; blockedTools switches specific ones off for
// this agent only. The legacy allowlist is cleared here so a user-facing agent
// is fully described by its denylist (the allowlist remains for subagent
// profiles, which never go through this endpoint).
func (d *DB) UpdateAgentTools(ctx context.Context, agentID string, mcpEnabled bool, blockedTools string) error {
	if blockedTools == "" {
		blockedTools = "[]"
	}
	_, err := d.mutateAgentLocked(agentID, func(a *Agent) {
		a.MCPEnabled = mcpEnabled
		a.BlockedTools = blockedTools
		a.AllowedTools = "[]"
	})
	return err
}

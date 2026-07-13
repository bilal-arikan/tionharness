package db

import (
	"context"
)

func (d *DB) persistMCPLocked(m MCPServer) error {
	return dbPersistLocked(d, d.mcp, dirMCP, m.ID, m)
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
	if m.HeadersConfig == "" {
		m.HeadersConfig = "{}"
	}
	if m.Scope == "" {
		m.Scope = "shared"
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return m, d.persistMCPLocked(m)
}

// UpdateMCPServer replaces the editable fields of an existing server, preserving
// its identity (ID, CreatedAt, CreatedBy) and Enabled state. Returns ErrNotFound
// if the id is unknown. A changed connection spec re-dials on the next turn via
// the pool's config fingerprint.
func (d *DB) UpdateMCPServer(ctx context.Context, id string, upd MCPServer) (MCPServer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	m, ok := d.mcp[id]
	if !ok {
		return MCPServer{}, ErrNotFound
	}
	m.Name = upd.Name
	m.Description = upd.Description
	m.Transport = upd.Transport
	m.Command = upd.Command
	m.Args = upd.Args
	m.URL = upd.URL
	m.EnvConfig = upd.EnvConfig
	m.HeadersConfig = upd.HeadersConfig
	m.Scope = upd.Scope
	// Same normalisation as create; identity + enabled are preserved above.
	if m.Transport == "" {
		m.Transport = MCPTransportStdio
	}
	if m.Args == "" {
		m.Args = "[]"
	}
	if m.EnvConfig == "" {
		m.EnvConfig = "{}"
	}
	if m.HeadersConfig == "" {
		m.HeadersConfig = "{}"
	}
	if m.Scope == "" {
		m.Scope = "shared"
	}
	return m, d.persistMCPLocked(m)
}

// GetMCPServer loads an MCP server by id.
func (d *DB) GetMCPServer(ctx context.Context, id string) (MCPServer, error) {
	return dbGet(d, d.mcp, id)
}

// ListMCPServers returns all configured servers, newest first.
func (d *DB) ListMCPServers(ctx context.Context) ([]MCPServer, error) {
	return dbList(d, d.mcp, func(a, b MCPServer) bool { return a.CreatedAt > b.CreatedAt }), nil
}

// ListEnabledMCPServers returns only servers with Enabled = true, newest first.
func (d *DB) ListEnabledMCPServers(ctx context.Context) ([]MCPServer, error) {
	return dbFilter(d, d.mcp,
		func(m MCPServer) bool { return m.Enabled },
		func(a, b MCPServer) bool { return a.CreatedAt > b.CreatedAt }), nil
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
	return dbDeleteLocked(d, d.mcp, dirMCP, id)
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

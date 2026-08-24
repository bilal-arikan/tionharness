package db

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
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

// TierBlocked is the ToolOverrides tier that drops a tool from an agent's
// catalog entirely — the successor to the standalone BlockedTools denylist.
// Declared here (not in the tools package) to avoid an import cycle; the value
// must stay in sync with agent.TierBlocked.
const TierBlocked = "blocked"

// UpdateAgentTools sets an agent's tool access: whether MCP tools are offered
// and the per-agent tool override map (JSON object, name/pattern → tier — the
// four visibility tiers plus "blocked"). Agents reach all workspace-active
// tools at the workspace-effective tier by default; this map overrides
// individual tools for this agent only.
//
// BlockedTools is DERIVED from the map's "blocked" entries and written along
// with it, so readers that predate the override map (market packs, workspace
// templates, agent files written by an older build) keep seeing an accurate
// denylist. The legacy allowlist is cleared so a user-facing agent is fully
// described by its overrides (the allowlist remains for subagent profiles,
// which never go through this endpoint).
func (d *DB) UpdateAgentTools(ctx context.Context, agentID string, mcpEnabled bool, toolOverrides string) error {
	if toolOverrides == "" {
		toolOverrides = "{}"
	}
	var overrides map[string]string
	if err := json.Unmarshal([]byte(toolOverrides), &overrides); err != nil {
		return fmt.Errorf("tool overrides must be a JSON object: %w", err)
	}
	blocked := []string{}
	for name, tier := range overrides {
		if tier == TierBlocked {
			blocked = append(blocked, name)
		}
	}
	sort.Strings(blocked) // stable on-disk order (map iteration is random)
	blockedJSON, err := json.Marshal(blocked)
	if err != nil {
		return err
	}
	_, err = d.mutateAgentLocked(agentID, func(a *Agent) {
		a.MCPEnabled = mcpEnabled
		a.ToolOverrides = toolOverrides
		a.BlockedTools = string(blockedJSON)
		a.AllowedTools = "[]"
	})
	return err
}

// UpdateAgentAllowedTools updates only a profile-managed legacy allowlist.
// Callers must first prove the stored value matches a known profile contract.
func (d *DB) UpdateAgentAllowedTools(ctx context.Context, agentID, allowedTools string) error {
	var allowed []string
	if err := json.Unmarshal([]byte(allowedTools), &allowed); err != nil {
		return fmt.Errorf("allowed tools must be a JSON array: %w", err)
	}
	_, err := d.mutateAgentLocked(agentID, func(a *Agent) {
		a.AllowedTools = allowedTools
	})
	return err
}

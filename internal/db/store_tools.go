package db

import "context"

// toolConfigFile is the singleton workspace-level tool activation document at
// the store root (one per workspace, since each workspace has its own store).
const toolConfigFile = "tools-config.json"

// WorkspaceToolConfig holds workspace-wide tool activation state. DisabledTools
// is a denylist of tool names switched off for the whole workspace; every tool
// not listed is active. A denylist is used (rather than an allowlist) so newly
// added tools (e.g. from a freshly enabled MCP server) default to active.
// HiddenTools is an independent set marking tools as load-on-demand (lazy): they
// stay active but their schemas are not shipped every turn — the agent pulls them
// in via tool_search / activate_tools. Mirrors a skill's auto-summary-off state.
type WorkspaceToolConfig struct {
	DisabledTools []string `json:"disabledTools"`
	HiddenTools   []string `json:"hiddenTools"`
	// ShownTools forces tools that are hidden/lazy by default (e.g. the
	// self-management suite, marked hidden in code) back into the every-turn
	// context. It overrides code defaults; an empty list keeps those defaults.
	ShownTools []string `json:"shownTools"`
}

// GetWorkspaceToolConfig returns a copy of the workspace tool config.
func (d *DB) GetWorkspaceToolConfig(ctx context.Context) (WorkspaceToolConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := WorkspaceToolConfig{
		DisabledTools: append([]string{}, d.toolConfig.DisabledTools...),
		HiddenTools:   append([]string{}, d.toolConfig.HiddenTools...),
		ShownTools:    append([]string{}, d.toolConfig.ShownTools...),
	}
	return out, nil
}

// SetWorkspaceToolConfig replaces and persists the workspace tool config.
func (d *DB) SetWorkspaceToolConfig(ctx context.Context, cfg WorkspaceToolConfig) error {
	if cfg.DisabledTools == nil {
		cfg.DisabledTools = []string{}
	}
	if cfg.HiddenTools == nil {
		cfg.HiddenTools = []string{}
	}
	if cfg.ShownTools == nil {
		cfg.ShownTools = []string{}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.toolConfig = cfg
	return atomicWriteJSON(d.dir(toolConfigFile), cfg)
}

// loadToolConfig reads the singleton tool config (absent = empty denylist).
func (d *DB) loadToolConfig() error {
	var cfg WorkspaceToolConfig
	if err := readJSONFile(d.dir(toolConfigFile), &cfg); err != nil {
		return nil // absent or unreadable → leave zero value (all active)
	}
	d.toolConfig = cfg
	return nil
}

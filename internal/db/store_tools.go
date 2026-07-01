package db

import "context"

// toolConfigFile is the singleton workspace-level tool activation document at
// the store root (one per workspace, since each workspace has its own store).
const toolConfigFile = "tools-config.json"

// WorkspaceToolConfig holds workspace-wide tool state.
//
//   - DisabledTools is a denylist of tool names switched off for the whole
//     workspace; every tool not listed is active. A denylist is used (rather than
//     an allowlist) so newly added tools (e.g. from a freshly enabled MCP server)
//     default to active.
//   - ToolVisibility is a per-tool override of how much of the tool rides in the
//     per-turn context: one of "full" | "summary" | "name-only" | "hidden" (see
//     tools.Visibility* constants). A tool absent from the map uses its code
//     default. This is independent of DisabledTools (a tool can be active yet
//     hidden, or disabled regardless of visibility).
//
// HiddenTools/ShownTools are the LEGACY two-list form (name-only / full); they are
// migrated into ToolVisibility on load and no longer written. Kept as fields only
// so an old config file still parses.
type WorkspaceToolConfig struct {
	DisabledTools  []string          `json:"disabledTools"`
	ToolVisibility map[string]string `json:"toolVisibility,omitempty"`

	// Deprecated: migrated into ToolVisibility on load. Not written back.
	HiddenTools []string `json:"hiddenTools,omitempty"`
	ShownTools  []string `json:"shownTools,omitempty"`
}

// Legacy → ToolVisibility mappings (kept here, not in the tools package, to avoid
// an import cycle; the string values must match tools.VisibilityNameOnly / Full).
const (
	visibilityNameOnly = "name-only"
	visibilityFull     = "full"
)

// GetWorkspaceToolConfig returns a copy of the workspace tool config with
// ToolVisibility populated (legacy lists already folded in by loadToolConfig).
func (d *DB) GetWorkspaceToolConfig(ctx context.Context) (WorkspaceToolConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	vis := make(map[string]string, len(d.toolConfig.ToolVisibility))
	for k, v := range d.toolConfig.ToolVisibility {
		vis[k] = v
	}
	out := WorkspaceToolConfig{
		DisabledTools:  append([]string{}, d.toolConfig.DisabledTools...),
		ToolVisibility: vis,
	}
	return out, nil
}

// SetWorkspaceToolConfig replaces and persists the workspace tool config. Legacy
// list fields are cleared on write — ToolVisibility is the sole source of truth.
func (d *DB) SetWorkspaceToolConfig(ctx context.Context, cfg WorkspaceToolConfig) error {
	if cfg.DisabledTools == nil {
		cfg.DisabledTools = []string{}
	}
	if cfg.ToolVisibility == nil {
		cfg.ToolVisibility = map[string]string{}
	}
	cfg.HiddenTools = nil
	cfg.ShownTools = nil
	d.mu.Lock()
	defer d.mu.Unlock()
	d.toolConfig = cfg
	return atomicWriteJSON(d.dir(toolConfigFile), cfg)
}

// loadToolConfig reads the singleton tool config (absent = empty denylist) and
// migrates any legacy HiddenTools/ShownTools lists into ToolVisibility so the rest
// of the code deals only with the map.
func (d *DB) loadToolConfig() error {
	var cfg WorkspaceToolConfig
	if err := readJSONFile(d.dir(toolConfigFile), &cfg); err != nil {
		return nil // absent or unreadable → leave zero value (all active)
	}
	if cfg.ToolVisibility == nil {
		cfg.ToolVisibility = map[string]string{}
	}
	// Migrate legacy lists. An explicit map entry wins over a legacy list entry.
	for _, n := range cfg.HiddenTools {
		if _, ok := cfg.ToolVisibility[n]; !ok {
			cfg.ToolVisibility[n] = visibilityNameOnly
		}
	}
	for _, n := range cfg.ShownTools {
		if _, ok := cfg.ToolVisibility[n]; !ok {
			cfg.ToolVisibility[n] = visibilityFull
		}
	}
	cfg.HiddenTools = nil
	cfg.ShownTools = nil
	d.toolConfig = cfg
	return nil
}

package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/mcp"
	"github.com/bilal/swarmgo/internal/tools"
)

// cliMCPConfig is the on-disk shape claude --mcp-config expects.
type cliMCPConfig struct {
	MCPServers map[string]cliMCPServer `json:"mcpServers"`
}

type cliMCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Type    string            `json:"type,omitempty"`    // sse | http
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"` // http transport (e.g. Authorization)
}

// interactionServerKey is the mcp-config key for the in-process Interaction MCP
// server; the CLI namespaces its tools as mcp__<key>__<tool>.
const interactionServerKey = "swarmgo_interaction"

// permissionPromptToolID is the namespaced Interaction MCP tool the claude CLI is
// pointed at via --permission-prompt-tool (only in "ask" mode) so risky tools are
// gated through SwarmGo's approval UI instead of auto-approved.
const permissionPromptToolID = "mcp__" + interactionServerKey + "__permission_prompt"

// promptToolForMode returns the permission-prompt tool id to hand the CLI, but
// only in "ask" mode with a live Interaction endpoint. Empty otherwise:
// read-only uses CLI plan mode and auto uses bypass — neither needs a per-tool
// prompt.
func promptToolForMode(mode string, inter tools.InteractionEndpoint) string {
	if mode == "ask" && inter.URL != "" {
		return permissionPromptToolID
	}
	return ""
}

// writeCLIMCPConfig renders the claude --mcp-config file for one turn and returns
// the path, the allowlist of tool identifiers, the list of CLI built-ins to
// disallow, and a cleanup func.
//
//   - When mcpEnabled, every enabled external MCP server is included.
//   - When inter.URL is set, the in-process Interaction MCP server is added so the
//     CLI can reach SwarmGo's human-in-the-loop tools (ask_user/todo_write), and
//     the conflicting CLI built-ins (AskUserQuestion/TodoWrite) are disallowed.
//
// Returns an empty path when there is nothing to wire.
func (r *Runtime) writeCLIMCPConfig(ctx context.Context, mcpEnabled bool, inter tools.InteractionEndpoint) (string, []string, []string, func(), error) {
	cfg := cliMCPConfig{MCPServers: map[string]cliMCPServer{}}
	var allowed, disallowed []string

	if mcpEnabled {
		servers, err := r.db.ListEnabledMCPServers(ctx)
		if err != nil {
			return "", nil, nil, nil, err
		}
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
	}

	if inter.URL != "" {
		cfg.MCPServers[interactionServerKey] = cliMCPServer{
			Type:    "http",
			URL:     inter.URL,
			Headers: map[string]string{"Authorization": "Bearer " + inter.Token},
		}
		// Allowlist derives from the names the backend advertises for this turn
		// (InteractionEndpoint.ToolNames) — a single source shared with Tools(), so
		// a tool added to the backend is allowlisted automatically.
		for _, t := range inter.ToolNames {
			allowed = append(allowed, "mcp__"+interactionServerKey+"__"+t)
		}
		// Suppress the CLI's own equivalents, which can't be answered/honored in
		// one-shot -p mode: AskUserQuestion/TodoWrite have no live client, and
		// ScheduleWakeup schedules a wake the CLI subprocess never lives to fire —
		// SwarmGo's own schedule_wake (above) replaces it with a real timer.
		disallowed = append(disallowed, "AskUserQuestion", "TodoWrite", "ScheduleWakeup")
	}

	if len(cfg.MCPServers) == 0 {
		return "", nil, nil, func() {}, nil
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", nil, nil, nil, err
	}
	f, err := os.CreateTemp("", "swarmgo-mcp-*.json")
	if err != nil {
		return "", nil, nil, nil, err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", nil, nil, nil, err
	}
	_ = f.Close()

	cleanup := func() { _ = os.Remove(path) }
	r.logger.Info("cli mcp config written", "path", filepath.Base(path),
		"servers", len(cfg.MCPServers), "interaction", inter.URL != "")
	return path, allowed, disallowed, cleanup, nil
}

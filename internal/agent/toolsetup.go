package agent

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/mcp"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// toServerConfig converts a stored MCP server row into a transport-agnostic
// launch spec for the mcp package.
func toServerConfig(m db.MCPServer) mcp.ServerConfig {
	var args []string
	_ = json.Unmarshal([]byte(m.Args), &args)
	env := map[string]string{}
	_ = json.Unmarshal([]byte(m.EnvConfig), &env)
	return mcp.ServerConfig{
		Name:      m.Name,
		Transport: m.Transport,
		Command:   m.Command,
		Args:      args,
		URL:       m.URL,
		Env:       env,
	}
}

// allowFunc builds a tool-name predicate from an agent's allowed_tools JSON
// allowlist. An empty list means "allow everything". A pattern ending in "*"
// matches by prefix; otherwise it matches exactly.
func allowFunc(agent db.Agent) func(string) bool {
	var patterns []string
	_ = json.Unmarshal([]byte(agent.AllowedTools), &patterns)
	if len(patterns) == 0 {
		return nil // nil predicate => allow all
	}
	return func(name string) bool {
		for _, p := range patterns {
			if strings.HasSuffix(p, "*") {
				if strings.HasPrefix(name, strings.TrimSuffix(p, "*")) {
					return true
				}
			} else if name == p {
				return true
			}
		}
		return false
	}
}

// buildRegistry assembles the tool registry for an agent: built-in tools plus
// the catalog of every enabled MCP server in the workspace. cfgByServer maps
// sanitized server names back to their configs for dispatch.
func (r *Runtime) buildRegistry(ctx context.Context, agent db.Agent) *tools.Registry {
	builtins := []tools.Tool{
		tools.TimeTool{},
		tools.NewHTTPGetTool(),
		tools.NewMemoryRecallTool(r.mem, agent.ID),
		// Interaction tools: todo_write surfaces a live checklist; ask_user pauses
		// the turn for a clarifying question (no-op outside interactive chat).
		tools.NewTodoWriteTool(),
		tools.NewAskUserTool(),
		// Artifact tools: save/revise substantial content as a versioned artifact
		// the user can open in a dedicated viewer (no-op outside interactive chat).
		tools.NewCreateArtifactTool(),
		tools.NewUpdateArtifactTool(),
	}

	// Workspace-scoped filesystem tools (sandboxed to this workspace's work dir).
	if sb := tools.NewSandbox(r.workDir); sb.Ready() {
		builtins = append(builtins,
			tools.NewFSReadFileTool(sb),
			tools.NewFSWriteFileTool(sb),
			tools.NewFSEditFileTool(sb),
			tools.NewFSListDirTool(sb),
			tools.NewFSGlobTool(sb),
			tools.NewFSGrepTool(sb),
		)
		// The shell tool is high-risk; offer it only when explicitly enabled.
		if r.tun.ShellEnabled() {
			builtins = append(builtins, tools.NewShellTool(sb))
		}
	}

	reg := tools.NewRegistry(builtins...)

	servers, err := r.db.ListEnabledMCPServers(ctx)
	if err != nil {
		r.logger.Warn("list mcp servers failed", "error", err)
		return reg
	}
	if len(servers) == 0 {
		return reg
	}

	cfgs := make([]mcp.ServerConfig, 0, len(servers))
	cfgByServer := map[string]mcp.ServerConfig{}
	for _, m := range servers {
		cfg := toServerConfig(m)
		cfgs = append(cfgs, cfg)
		// Key by the sanitized name used in namespacing.
		srv, _, _ := mcp.SplitNamespaced(mcp.NamespaceTool(cfg.Name, "x"))
		cfgByServer[srv] = cfg
	}

	entries, errs := mcp.BuildCatalog(ctx, cfgs)
	for name, e := range errs {
		r.logger.Warn("mcp catalog build failed", "server", name, "error", e)
	}
	reg.AttachMCP(entries, cfgByServer)
	return reg
}

// workspaceDisabledSet loads the workspace-level tool denylist as a set. Tools
// in this set are switched off for the whole workspace regardless of per-agent
// selection. A nil/empty set means everything is active.
func (r *Runtime) workspaceDisabledSet(ctx context.Context) map[string]bool {
	cfg, err := r.db.GetWorkspaceToolConfig(ctx)
	if err != nil || len(cfg.DisabledTools) == 0 {
		return nil
	}
	set := make(map[string]bool, len(cfg.DisabledTools))
	for _, n := range cfg.DisabledTools {
		set[n] = true
	}
	return set
}

// toolFilter is the effective tool predicate for an agent: a tool is offered
// only when it is active at the workspace level AND permitted by the agent's
// own allowlist (empty allowlist = all workspace-active tools).
func (r *Runtime) toolFilter(ctx context.Context, agent db.Agent) func(string) bool {
	disabled := r.workspaceDisabledSet(ctx)
	agentAllow := allowFunc(agent) // nil => agent allows all
	if disabled == nil && agentAllow == nil {
		return nil
	}
	return func(name string) bool {
		if disabled[name] {
			return false
		}
		return agentAllow == nil || agentAllow(name)
	}
}

// ToolCatalog returns the effective tool catalog for an agent (workspace-active
// AND permitted by the agent's allowlist) — the tools it actually receives.
func (r *Runtime) ToolCatalog(ctx context.Context, agent db.Agent) []providers.ToolDef {
	return r.buildRegistry(ctx, agent).Defs(r.toolFilter(ctx, agent))
}

// WorkspaceToolCatalog returns the full, unfiltered tool catalog (every built-in
// plus every enabled MCP server's tools) for the workspace tools screen, where
// each tool's active/inactive state is toggled independently of any agent.
func (r *Runtime) WorkspaceToolCatalog(ctx context.Context) []providers.ToolDef {
	return r.buildRegistry(ctx, db.Agent{}).Defs(nil)
}

// ActiveToolCatalog returns the workspace-active tool catalog (full catalog
// minus the workspace denylist) — the set of tools an agent may pick from.
func (r *Runtime) ActiveToolCatalog(ctx context.Context) []providers.ToolDef {
	disabled := r.workspaceDisabledSet(ctx)
	if disabled == nil {
		return r.buildRegistry(ctx, db.Agent{}).Defs(nil)
	}
	return r.buildRegistry(ctx, db.Agent{}).Defs(func(name string) bool { return !disabled[name] })
}

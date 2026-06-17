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
		// the turn for a clarifying question; request_confirmation blocks for a
		// yes/no on a risky action (all no-ops outside interactive chat).
		tools.NewTodoWriteTool(),
		tools.NewAskUserTool(),
		tools.NewRequestConfirmationTool(),
		// Artifact tools: save/revise substantial content as a versioned artifact
		// the user can open in a dedicated viewer (no-op outside interactive chat).
		tools.NewCreateArtifactTool(),
		tools.NewUpdateArtifactTool(),
	}

	// Skills: when the workspace has any skill, offer use_skill so the agent can
	// load a skill's full instructions on demand (the catalog is advertised in
	// the system prompt; bodies stay on disk until invoked — lazy loading).
	if r.skills != nil && !r.skills.Empty() {
		builtins = append(builtins, tools.NewUseSkillTool(r.skills))
	}

	// Agent→agent delegation: the call_agent tool lets this agent hand a sub-task
	// to another agent and wait for its reply. Gated (off by default) because it
	// multiplies token cost and lets one turn fan out across several agents; the
	// runner enforces depth/cycle/budget guards.
	if r.tun.DelegationEnabled() {
		builtins = append(builtins, tools.NewCallAgentTool())
	}

	// Cross-session awareness: the list_sessions pull tool (complements the pushed
	// context block). Gated per-workspace by the same master toggle.
	if r.SessionContextEnabled() {
		builtins = append(builtins, tools.NewListSessionsTool(r.db))
	}

	// Workspace secret vault: let agents discover and fetch stored credentials
	// (API keys, tokens, passwords) for the tasks they run.
	if r.vault != nil {
		builtins = append(builtins,
			tools.NewSecretListTool(r.vault),
			tools.NewSecretGetTool(r.vault),
		)
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

	// Workspace config tools (sandboxed to <workspace>/config/): let an agent
	// read and edit its OWN runtime prompts / instructions / README.
	if cb := tools.NewSandbox(r.configDir()); cb.Ready() {
		builtins = append(builtins,
			tools.NewConfigReadTool(cb),
			tools.NewConfigWriteTool(cb),
			tools.NewConfigListTool(cb),
		)
	}

	// Self-management suite (gated, off by default): let an agent create/edit/
	// delete agents, flows and schedules, manage artifacts, add memories and read
	// logs. Provenance is enforced — agents only touch agent-created entities.
	// This roughly doubles the tool catalog, so it is opt-in per workspace.
	if r.tun.SelfManageEnabled() {
		builtins = append(builtins,
			// Agents.
			tools.NewCreateAgentTool(r.db, agent.ID, r.Start),
			tools.NewUpdateAgentTool(r.db, agent.ID),
			tools.NewDeleteAgentTool(r.db, agent.ID, r.Stop),
			tools.NewListAgentsTool(r.db, agent.ID),
			// Flows.
			tools.NewCreateFlowTool(r.db, agent.ID),
			tools.NewUpdateFlowTool(r.db, agent.ID),
			tools.NewDeleteFlowTool(r.db, agent.ID),
			tools.NewListFlowsTool(r.db, agent.ID),
			// Schedules (routines).
			tools.NewCreateScheduleTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewUpdateScheduleTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewDeleteScheduleTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewListSchedulesTool(r.db, agent.ID),
			// Tasks (kanban board). Read/create/edit/move/run on any task;
			// delete only agent-created (provenance).
			tools.NewListTasksTool(r.db, agent.ID),
			tools.NewCreateTaskTool(r.db, agent.ID),
			tools.NewUpdateTaskTool(r.db, agent.ID),
			tools.NewMoveTaskTool(r.db, agent.ID),
			tools.NewRunTaskTool(r.db, agent.ID, r.RunTask),
			tools.NewDeleteTaskTool(r.db, agent.ID),
			// Artifacts (create/update already provided via the per-turn sink).
			tools.NewDeleteArtifactTool(r.db, agent.ID),
			tools.NewListArtifactsTool(r.db, agent.ID),
			// Memory (recall already provided above) + logs.
			tools.NewMemoryAddTool(r.mem, agent.ID),
			tools.NewReadLogsTool(r.logs),
		)
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

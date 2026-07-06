package tools

// Functional categories for built-in tools, used by the workspace tools screen to
// group an otherwise-flat list (~85 built-ins) into navigable sections. The key is
// a stable English identifier; the UI maps it to a localized label. MCP tools are
// NOT categorized here — they group by their originating server instead.
//
// This is the single source of truth: when a new built-in tool is added, list it
// here. Anything not listed falls into CategoryOther so it stays visible (never
// silently dropped) and is easy to spot as "uncategorized".
const (
	CategoryFiles       = "files"       // filesystem + shell
	CategorySearch      = "search"      // web + conversation search
	CategoryAgents      = "agents"      // agents, subagents, sessions, delegation
	CategoryAutomation  = "automation"  // flows, schedules, tasks, hooks
	CategoryInteraction = "interaction" // user-facing prompts/notifications
	CategoryArtifacts   = "artifacts"   // artifact CRUD
	CategorySkillsMCP   = "skills-mcp"  // skills + MCP server management
	CategoryConfig      = "config"      // settings, config, workspaces, secrets
	CategoryDiagnostics = "diagnostics" // logs, debug, validators, tool loading
	CategoryOther       = "other"       // fallback for unlisted tools
)

// builtinCategory maps each built-in tool NAME to its functional category. Kept as
// one explicit map (rather than name-prefix heuristics) so the grouping is exact
// and reviewable.
var builtinCategory = map[string]string{
	// Files & shell
	"Read": CategoryFiles, "Write": CategoryFiles, "Edit": CategoryFiles,
	"LS": CategoryFiles, "Glob": CategoryFiles, "Grep": CategoryFiles,
	"Bash": CategoryFiles, "PowerShell": CategoryFiles,
	"shell_manage": CategoryFiles,

	// Search & web
	"WebFetch": CategorySearch, "WebSearch": CategorySearch,
	"conversation_search":       CategorySearch,
	"codebase_workspace_search": CategorySearch,

	// Agents, subagents, sessions, delegation
	"create_agent": CategoryAgents, "update_agent": CategoryAgents,
	"delete_agent": CategoryAgents, "list_agents": CategoryAgents,
	"run_subagent": CategoryAgents, "spawn_session": CategoryAgents,
	"handoff_session": CategoryAgents, "send_message": CategoryAgents,
	"list_sessions": CategoryAgents, "update_session": CategoryAgents,
	"archive_sessions": CategoryAgents,
	"focus_view":       CategoryAgents, "get_session_info": CategoryAgents,

	// User profile (Settings ▸ Profile) — grouped with settings/config.
	"update_user_preferences": CategoryConfig,

	// Automation: flows, schedules, tasks, hooks
	"create_flow": CategoryAutomation, "update_flow": CategoryAutomation,
	"delete_flow": CategoryAutomation, "list_flows": CategoryAutomation,
	"get_flow": CategoryAutomation, "run_flow": CategoryAutomation,
	"run_schedule": CategoryAutomation, "create_schedule": CategoryAutomation,
	"update_schedule": CategoryAutomation, "delete_schedule": CategoryAutomation,
	"list_schedules": CategoryAutomation, "schedule_wake": CategoryAutomation,
	"list_tasks": CategoryAutomation, "create_task": CategoryAutomation,
	"update_task": CategoryAutomation, "move_task": CategoryAutomation,
	"delete_task": CategoryAutomation, "todo_write": CategoryAutomation,
	"list_hooks": CategoryAutomation, "create_hook": CategoryAutomation,
	"delete_hook": CategoryAutomation,

	// Interaction
	"ask_user": CategoryInteraction, "request_confirmation": CategoryInteraction,
	"notify": CategoryInteraction,

	// Artifacts
	"create_artifact": CategoryArtifacts, "update_artifact": CategoryArtifacts,
	"read_artifact": CategoryArtifacts, "delete_artifact": CategoryArtifacts,
	"list_artifacts": CategoryArtifacts,

	// Skills & MCP management
	"use_skill": CategorySkillsMCP, "create_skill": CategorySkillsMCP,
	"update_skill": CategorySkillsMCP, "delete_skill": CategorySkillsMCP,
	"import_skill": CategorySkillsMCP, "skill_search": CategorySkillsMCP,
	"skill_validate": CategorySkillsMCP, "list_mcp_servers": CategorySkillsMCP,
	"create_mcp_server": CategorySkillsMCP, "toggle_mcp_server": CategorySkillsMCP,
	"delete_mcp_server": CategorySkillsMCP,

	// Settings, config, workspaces, secrets
	"get_settings": CategoryConfig, "update_settings": CategoryConfig,
	"read_config": CategoryConfig, "write_config": CategoryConfig,
	"list_config": CategoryConfig, "config_validate": CategoryConfig,
	"list_workspaces": CategoryConfig, "create_workspace": CategoryConfig,
	"rename_workspace": CategoryConfig, "delete_workspace": CategoryConfig,
	"secret": CategoryConfig,

	// Diagnostics, validators, tool loading
	"read_logs": CategoryDiagnostics, "read_session_debug": CategoryDiagnostics,
	"mermaid_validate": CategoryDiagnostics, "transform_data": CategoryDiagnostics,
	"activate_tools": CategoryDiagnostics, "deactivate_tools": CategoryDiagnostics,
	"tool_search": CategoryDiagnostics,
}

// CategoryOf returns the functional category key for a built-in tool name, or
// CategoryOther when the tool is not explicitly mapped. MCP tools (namespaced) are
// not classified here; callers group those by server.
func CategoryOf(name string) string {
	if c, ok := builtinCategory[name]; ok {
		return c
	}
	return CategoryOther
}

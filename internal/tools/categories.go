package tools

import "strings"

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
	"apply_patch":  CategoryFiles, "run_code": CategoryFiles,

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
	// Coordinator/worker delegation (a coordinator drives background workers;
	// a worker reports back) — same family as run_subagent/spawn_session.
	"spawn_worker": CategoryAgents, "send_to_worker": CategoryAgents,
	"stop_worker": CategoryAgents, "list_workers": CategoryAgents,
	"report_to_coordinator": CategoryAgents, "set_coordinator_mode": CategoryAgents,

	// User profile (Settings ▸ Profile) — grouped with settings/config.
	"update_user_preferences": CategoryConfig,

	// Automation: flows, schedules, tasks, hooks
	"create_flow": CategoryAutomation, "update_flow": CategoryAutomation,
	"delete_flow": CategoryAutomation, "list_flows": CategoryAutomation,
	"get_flow": CategoryAutomation, "run_flow": CategoryAutomation,
	"list_flow_runs": CategoryAutomation, "deliver_flow_input": CategoryAutomation,
	"run_schedule": CategoryAutomation, "create_schedule": CategoryAutomation,
	"update_schedule": CategoryAutomation, "delete_schedule": CategoryAutomation,
	"list_schedules": CategoryAutomation, "schedule_wake": CategoryAutomation,
	"list_tasks": CategoryAutomation, "get_task": CategoryAutomation, "create_task": CategoryAutomation,
	"update_task": CategoryAutomation, "move_task": CategoryAutomation,
	"set_archived_task": CategoryAutomation, "delete_task": CategoryAutomation, "todo_write": CategoryAutomation,
	"list_hooks": CategoryAutomation, "create_hook": CategoryAutomation,
	"update_hook": CategoryAutomation, "delete_hook": CategoryAutomation,
	"list_automations": CategoryAutomation, "create_automation": CategoryAutomation,
	"update_automation": CategoryAutomation, "delete_automation": CategoryAutomation,

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
	"secret": CategoryConfig, "list_providers": CategoryConfig,

	// Diagnostics, validators, tool loading
	"read_logs": CategoryDiagnostics, "read_session_debug": CategoryDiagnostics,
	"get_view":         CategoryDiagnostics,
	"expand":           CategoryDiagnostics,
	"mermaid_validate": CategoryDiagnostics, "transform_data": CategoryDiagnostics,
	"render_template": CategoryDiagnostics,
	"activate_tools":  CategoryDiagnostics, "deactivate_tools": CategoryDiagnostics,
	"tool_search": CategoryDiagnostics,
	// Retrospective self-improvement: scan findings and the error→lesson store.
	"insight_scan": CategoryDiagnostics, "insight_list_findings": CategoryDiagnostics,
	"insight_apply_finding": CategoryDiagnostics,
	"read_lessons":          CategoryDiagnostics, "delete_lesson": CategoryDiagnostics,
}

// GroupPrefix marks an override key as a GROUP key rather than a tool name:
// "group:files" bans every built-in tool in the "files" category at once. It is
// deliberately not a valid tool-name character sequence, so a group key can
// never collide with a real tool or with a "prefix*" pattern.
const GroupPrefix = "group:"

// nsSep is the MCP namespace separator. A tool name containing it is namespaced
// (an MCP tool) and therefore belongs to its SERVER group, not to a functional
// category — mirrors mcp.SplitNamespaced without importing internal/mcp (which
// would be an import cycle from the tools package).
const nsSep = "__"

// orderedCategories is the stable presentation order of the functional
// categories: general-purpose work first, plumbing last. Callers rely on the
// order being deterministic (API payloads, UI rows).
var orderedCategories = []string{
	CategoryFiles, CategorySearch, CategoryAgents, CategoryAutomation,
	CategoryInteraction, CategoryArtifacts, CategorySkillsMCP, CategoryConfig,
	CategoryDiagnostics, CategoryOther,
}

// Categories returns every functional category key in stable order.
func Categories() []string {
	out := make([]string, len(orderedCategories))
	copy(out, orderedCategories)
	return out
}

// IsGroupKey reports whether an override key is a group key.
func IsGroupKey(key string) bool {
	return len(key) > len(GroupPrefix) && key[:len(GroupPrefix)] == GroupPrefix
}

// ValidGroupKey reports whether key is a group key naming a KNOWN category. An
// unknown category is rejected by callers rather than silently matching nothing.
func ValidGroupKey(key string) bool {
	if !IsGroupKey(key) {
		return false
	}
	cat := key[len(GroupPrefix):]
	for _, c := range orderedCategories {
		if c == cat {
			return true
		}
	}
	return false
}

// MatchesGroup reports whether toolName belongs to the group named by groupKey.
// Only non-namespaced BUILT-IN tools can match: MCP tools group by server and
// are targeted with the existing "<ns>__*" prefix pattern instead.
func MatchesGroup(toolName, groupKey string) bool {
	if !ValidGroupKey(groupKey) {
		return false
	}
	if strings.Contains(toolName, nsSep) {
		return false
	}
	return CategoryOf(toolName) == groupKey[len(GroupPrefix):]
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

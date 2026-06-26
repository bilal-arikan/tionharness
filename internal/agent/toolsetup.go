package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/mcp"
	"github.com/bilal-arikan/swarmgo/internal/providers"
	"github.com/bilal-arikan/swarmgo/internal/skills"
	"github.com/bilal-arikan/swarmgo/internal/tools"
)

// skillExists reports whether a skill slug is known to this workspace (global or
// workspace tier). Permissive when no skill store is wired (bare test runtimes).
func (r *Runtime) skillExists(slug string) bool {
	if r.skills == nil {
		return true
	}
	_, ok := r.skills.Get(slug)
	return ok
}

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

// patternPredicate compiles a list of tool-name patterns into a matcher. A
// pattern ending in "*" matches by prefix; otherwise it matches exactly. An
// empty list yields a nil predicate (caller treats nil as "no constraint").
func patternPredicate(patterns []string) func(string) bool {
	if len(patterns) == 0 {
		return nil
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

// allowFunc builds a tool-name predicate from an agent's allowed_tools JSON
// allowlist. An empty list means "allow everything" (nil predicate). This is the
// legacy allowlist used by built-in subagent profiles; user-facing agents leave
// it empty and rely on the denylist (blockFunc) instead.
func allowFunc(agent db.Agent) func(string) bool {
	var patterns []string
	_ = json.Unmarshal([]byte(agent.AllowedTools), &patterns)
	return patternPredicate(patterns)
}

// blockFunc builds a tool-name predicate from an agent's blocked_tools JSON
// denylist — it reports whether a tool is BLOCKED for this agent. An empty list
// means "nothing blocked" (nil predicate). This is the per-agent denylist driven
// from agent detail: agents reach all tools by default, minus these.
func blockFunc(agent db.Agent) func(string) bool {
	var patterns []string
	_ = json.Unmarshal([]byte(agent.BlockedTools), &patterns)
	return patternPredicate(patterns)
}

// buildRegistry assembles the tool registry for an agent: built-in tools plus
// the catalog of every enabled MCP server in the workspace. cfgByServer maps
// sanitized server names back to their configs for dispatch.
func (r *Runtime) buildRegistry(ctx context.Context, agent db.Agent) *tools.Registry {
	builtins := []tools.Tool{
		tools.NewWebFetchTool(),
		tools.NewMemoryRecallTool(r.mem, agent.ID),
		// Interaction tools: todo_write surfaces a live checklist; ask_user pauses
		// the turn for a clarifying question; request_confirmation blocks for a
		// yes/no on a risky action (all no-ops outside interactive chat).
		tools.NewTodoWriteTool(),
		tools.NewAskUserTool(),
		tools.NewRequestConfirmationTool(),
		// schedule_wake: pause and have the agent re-invoked after a delay to
		// continue the conversation (no-op outside interactive chat — the wake
		// scheduler is only wired onto a chat turn's context).
		tools.NewScheduleWakeTool(),
		// Artifact tools: save/revise substantial content as a versioned artifact
		// the user can open in a dedicated viewer (no-op outside interactive chat).
		tools.NewCreateArtifactTool(),
		tools.NewUpdateArtifactTool(),
		// notify: raise a non-blocking desktop notification to get the user's
		// attention (no-op outside interactive chat — the sink is only wired onto a
		// chat turn's context).
		tools.NewNotifyTool(),
		// focus_view: drive the user's UI to a screen/entity to direct attention
		// (no-op outside interactive chat — the navigate sink is only wired onto a
		// chat turn's context).
		tools.NewFocusViewTool(),
		// Session goal: set/complete this session's persistent "north star" — the
		// SAME field the user edits in the UI, injected into every turn (no-op
		// without a goal sink, i.e. outside a session-bound turn).
		tools.NewSetSessionGoalTool(),
		tools.NewCompleteGoalTool(),
		// Session edit: rename / set working dir / archive THIS session (no-op
		// without a session sink, i.e. outside a session-bound turn).
		tools.NewSetSessionTitleTool(),
		tools.NewSetWorkingDirTool(),
		tools.NewArchiveSessionTool(),
	}

	// Core memory (MemGPT-style): the agent edits its own persistent working-memory
	// block, re-injected into every prompt by composeTurnRequest. Opt-in per
	// settings (on by default) so a minimal-surface workspace can drop the two
	// tools. Eager: the model should reach for them readily as facts change.
	if r.tun.CoreMemoryTools() {
		builtins = append(builtins,
			tools.NewCoreMemoryReplaceTool(r.mem, agent.ID),
			tools.NewCoreMemoryAppendTool(r.mem, agent.ID),
		)
	}

	// Skills: an agent may load its ASSIGNED skills plus every SHARED (on-demand)
	// skill. Their summaries are advertised in the system prompt; bodies stay on
	// disk until use_skill is called (lazy). The tool is restricted to that set —
	// a restricted skill is unreachable unless explicitly assigned.
	if r.skills != nil {
		if allow := r.skills.AllowedFor(agent.Skills); len(allow) > 0 {
			lib := agentSkillLib{store: r.skills, allow: allow}
			builtins = append(builtins, tools.NewUseSkillTool(lib))
			// SK-2: skill_search lets the agent discover on-demand/conditional skills
			// that are deliberately kept out of the per-turn catalog.
			builtins = append(builtins, tools.NewSkillSearchTool(lib))
		}
	}

	// Subagents: the generic run_subagent tool launches an isolated worker — a
	// built-in profile (explore/coder/reviewer) or an existing agent — sync or
	// async, gathering only its final result so a sub-task's tool output never
	// floods this turn's context. Gated (off by default) because it multiplies
	// token cost and lets one turn fan out across several subagents; the runner
	// enforces depth/cycle/budget/concurrency guards. Eager (always shipped) so the
	// model reaches for it readily.
	if r.tun.DelegationEnabled() {
		builtins = append(builtins, tools.NewRunSubagentTool())
	}

	// Cross-session awareness: the list_sessions pull tool (complements the pushed
	// context block). Gated per-workspace by the same master toggle.
	if r.SessionContextEnabled() {
		builtins = append(builtins, tools.NewListSessionsTool(r.db))
		// conversation_search: full-text search across the workspace's message
		// history (deeper than list_sessions' titles+summaries). Same gate.
		builtins = append(builtins, tools.NewConversationSearchTool(r.db))
	}

	// read_session_debug: the agent reads its OWN session's structured debug
	// journal (turn timings, token spend, tool latency/errors, anomalies) to
	// self-diagnose and optimise. Always-on (read-only, the sibling of
	// conversation_search for self-improvement) whenever the debug journal is on,
	// so it works out of the box — including for the default keyless claude-cli
	// agent — rather than being buried in the hidden-lazy self-manage tier.
	if r.tun != nil && r.tun.DebugJournalEnabled() {
		builtins = append(builtins, tools.NewReadSessionDebugTool(r.db))
	}

	// Workspace secret vault: let agents discover and fetch stored credentials
	// (API keys, tokens, passwords) for the tasks they run.
	if r.vault != nil {
		builtins = append(builtins,
			tools.NewSecretListTool(r.vault),
			tools.NewSecretGetTool(r.vault),
		)
	}

	// Filesystem tools, rooted at this turn's working directory (the session's
	// WorkingDir override, else the workspace default). They are UNCONFINED by
	// default (may touch any path); autonomous turns re-confine them to the working
	// dir when AutonomousConfine is on — the safety brake. The working dir + the
	// autonomous flag arrive via ctx (resolvedWorkDirFromCtx); catalog/preview
	// calls without it fall back to the workspace default, unconfined.
	wd := r.workDir
	confine := false
	if rw, ok := resolvedWorkDirFromCtx(ctx); ok {
		wd = rw.dir
		confine = rw.autonomous && r.tun.AutonomousConfine()
	}
	sb := tools.NewSandbox(wd)
	if confine {
		sb = tools.NewConfinedSandbox(wd)
	}
	if sb.Ready() {
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

	// Workspace config tools (confined to <workspace>/config/): let an agent
	// read and edit its OWN runtime prompts / instructions / README. This stays a
	// hard confinement boundary even though the workspace fs/shell sandbox is
	// unconfined — the config tool must not reach outside the agent's own config.
	if cb := tools.NewConfinedSandbox(r.configDir()); cb.Ready() {
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
	selfManageStart := len(builtins)
	if r.tun.SelfManageEnabled() {
		builtins = append(builtins,
			// Agents. New agents are seeded with the default SwarmGo skill set when
			// the caller passes none; caller-supplied slugs are validated against the
			// skill store.
			tools.NewCreateAgentTool(r.db, agent.ID, skills.DefaultSkillSlugs(), r.skillExists),
			tools.NewUpdateAgentTool(r.db, agent.ID),
			tools.NewDeleteAgentTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewListAgentsTool(r.db, agent.ID),
			// Note: agent→agent work is unified under run_subagent (above) — async
			// background runs go through its wait:"async" mode (→ SpawnSession). The
			// old call_agent / spawn_session / send_agent_message tools were removed.
			// send_message is the "peer DM" complement: an addressed, sender-tagged
			// message into another agent's persistent inbox (Claude Code mailbox model).
			tools.NewSendMessageTool(agent.ID, func(ctx context.Context, to, summary, message string) (string, error) {
				return r.DeliverAgentMessage(ctx, agent.ID, to, summary, message)
			}),
			// Context reset: let the agent hand off to a fresh session when it nears
			// the context limit (Anthropic "context reset" pattern) — operates on the
			// CURRENT session (resolved from the context) and the running agent.
			tools.NewHandoffSessionTool(func(ctx context.Context, reason string) (tools.HandoffResult, error) {
				sid := SessionIDFrom(ctx)
				if sid == "" {
					return tools.HandoffResult{}, fmt.Errorf("no active session to hand off")
				}
				sess, err := r.db.GetSession(ctx, sid)
				if err != nil {
					return tools.HandoffResult{}, err
				}
				res, err := r.HandoffSession(ctx, sess, agent, HandoffOptions{Reason: HandoffReasonAgent, CreatedBy: agent.ID})
				if err != nil {
					return tools.HandoffResult{}, err
				}
				return tools.HandoffResult{NewSessionID: res.NewSessionID, AgentName: res.AgentName, ArtifactID: res.ArtifactID}, nil
			}),
			// Flows.
			tools.NewCreateFlowTool(r.db, agent.ID),
			tools.NewUpdateFlowTool(r.db, agent.ID),
			tools.NewDeleteFlowTool(r.db, agent.ID),
			tools.NewListFlowsTool(r.db, agent.ID),
			tools.NewGetFlowTool(r.db, agent.ID),
			// run_flow drives a flow to completion (autonomous, budget-gated) and
			// records it in the executions feed, like run_task.
			tools.NewRunFlowTool(r.db, agent.ID, func(ctx context.Context, flowID, input string) (db.FlowRun, error) {
				run, _, err := r.RunFlowRecorded(ctx, flowID, input, true, nil)
				return run, err
			}),
			// Schedules (routines).
			tools.NewCreateScheduleTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewUpdateScheduleTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewDeleteScheduleTool(r.db, agent.ID, r.reloadSchedules),
			tools.NewListSchedulesTool(r.db, agent.ID),
			// Manually fire a schedule now (the "Run now" trigger).
			tools.NewRunScheduleTool(r.db, r.runScheduleNow),
			// Tasks (kanban board). Read/create/edit/move on any task; delete only
			// agent-created (provenance). The board is passive — no run tool.
			tools.NewListTasksTool(r.db, agent.ID),
			tools.NewCreateTaskTool(r.db, agent.ID),
			tools.NewUpdateTaskTool(r.db, agent.ID),
			tools.NewMoveTaskTool(r.db, agent.ID),
			tools.NewDeleteTaskTool(r.db, agent.ID),
			// Hooks (PreToolUse/PostToolUse). List/create on any; delete only
			// agent-created (provenance).
			tools.NewListHooksTool(r.db, agent.ID),
			tools.NewCreateHookTool(r.db, agent.ID),
			tools.NewDeleteHookTool(r.db, agent.ID),
			// MCP servers. List/create/toggle on any; delete only agent-created
			// (provenance). New/enabled servers are picked up next turn.
			tools.NewListMCPServersTool(r.db, agent.ID),
			tools.NewCreateMCPServerTool(r.db, agent.ID),
			tools.NewToggleMCPServerTool(r.db, agent.ID),
			tools.NewDeleteMCPServerTool(r.db, agent.ID),
			// Artifacts (create/update already provided via the per-turn sink).
			tools.NewDeleteArtifactTool(r.db, agent.ID),
			tools.NewListArtifactsTool(r.db, agent.ID),
			tools.NewReadArtifactTool(r.db, agent.ID),
			// Memory (recall already provided above) + logs.
			tools.NewMemoryAddTool(r.mem, agent.ID),
			tools.NewReadLogsTool(r.logs),
		)
		// Secret vault writes: store/remove credentials (read is always-on above).
		if r.vault != nil {
			builtins = append(builtins,
				tools.NewSecretSetTool(r.vault),
				tools.NewSecretDeleteTool(r.vault),
			)
		}
		// Skill authoring: create/update/delete/import reusable workspace skills.
		if r.skills != nil {
			builtins = append(builtins,
				tools.NewCreateSkillTool(agentSkillWriter{store: r.skills}),
				tools.NewUpdateSkillTool(agentSkillWriter{store: r.skills}),
				tools.NewDeleteSkillTool(agentSkillWriter{store: r.skills, db: r.db}),
				tools.NewImportSkillTool(agentSkillWriter{store: r.skills}), // SK-IMP
			)
		}
		// Application-wide settings: read + live-apply the settings.json document
		// behind the Settings screen. Only offered when the bridge is wired (the
		// api server provides it), since changes affect every workspace.
		if r.settingsBridge != nil {
			builtins = append(builtins,
				tools.NewGetSettingsTool(r.settingsBridge),
				tools.NewUpdateSettingsTool(r.settingsBridge),
			)
		}
		// Workspaces: list/create/rename across all workspaces; delete only
		// agent-created ones (and never the current or last). Cross-workspace, so
		// it goes through the bridge the manager wires from the api server. Only
		// offered when that bridge is present.
		if r.workspaceBridge != nil {
			builtins = append(builtins,
				tools.NewListWorkspacesTool(r.workspaceBridge, agent.ID, r.wsID),
				tools.NewCreateWorkspaceTool(r.workspaceBridge, agent.ID, r.wsID),
				tools.NewRenameWorkspaceTool(r.workspaceBridge, agent.ID, r.wsID),
				tools.NewDeleteWorkspaceTool(r.workspaceBridge, agent.ID, r.wsID),
			)
		}
	}

	reg := tools.NewRegistry(builtins...)

	// Lazy tool loading: the self-management suite is large and used in a minority
	// of turns, so its schemas are loaded on demand (activate_tools) rather than
	// shipped every turn. MCP tools are marked lazy inside AttachMCP.
	//
	// HIDDEN: the self-management family is also kept OUT of the rendered
	// load-on-demand catalog block — dozens of name+summary lines would otherwise
	// ride in every turn's cached prefix. Their catalog + usage lives in the
	// `swarmgo-self-management` skill (advertised in Available Skills); the block
	// shows a single pointer to it. They stay activatable (activate_tools) and
	// searchable (tool_search), so the skill is the documented path, not the only one.
	for _, t := range builtins[selfManageStart:] {
		reg.MarkHidden(t.Def().Name)
	}
	// Default NAME-ONLY tier: a curated set of always-built tools that are
	// self-descriptive AND used in only a minority of turns. They are listed in the
	// load-on-demand catalog by NAME ALONE (no schema, no summary) — the Claude Code
	// "deferred tool" style: the model recognises them by name and pulls the schema
	// via tool_search / activate_tools when it actually needs one. This strips their
	// schemas (and, for the formerly-eager ones, their per-turn cost entirely) from
	// EVERY turn's cached prefix, while keeping them fully reachable. On the
	// claude-cli path the Interaction MCP bridge still advertises the bridgeable ones
	// with full schemas, so capability is unchanged there. MarkNameOnly on a name not
	// present in this agent's builtins is a harmless no-op, so gated tools
	// (vault/config/session-context off) need no extra guarding here.
	//
	// Deliberately kept EAGER (behavioral nudges or high-frequency): todo_write,
	// ask_user, request_confirmation, create_artifact/update_artifact,
	// core_memory_*, use_skill/skill_search, run_subagent, Read/Write/Edit/list_dir/
	// Glob/Grep, shell. The self-management suite stays MarkHidden (dropped from the
	// catalog entirely — more aggressive than name-only).
	reg.MarkNameOnly(
		// Session lifecycle & navigation — names say it all; rarely the turn's point.
		"set_session_goal", "complete_goal",
		"set_session_title", "set_working_dir", "archive_session",
		"notify", "focus_view", "schedule_wake",
		// Cross-session & self-diagnostics — occasional, discoverable by name.
		"list_sessions", "conversation_search", "read_session_debug",
		// Memory recall — recall is already auto-injected via ContextBlock.
		"memory_recall",
		// Artifact revise + meta — create_artifact stays eager (behavioral); revise
		// and the deactivate meta-tool are reached on demand.
		"update_artifact", "deactivate_tools",
		// Promoted out of the hidden self-management group: common enough to advertise
		// by name (handoff at context limit, add a memory, DM a peer agent) rather than
		// fold into the self-management skill pointer. MarkNameOnly clears the earlier
		// MarkHidden on these (disjoint tiers, last mark wins).
		"handoff_session", "memory_add", "send_message",
	)
	// Admin-rare tools fold into the HIDDEN self-management group (not enumerated
	// per turn — surfaced via the swarmgo-self-management skill / tool_search). These
	// are confined config edits and secret reads: used in a tiny fraction of turns,
	// and their WRITE siblings (secret_set/secret_delete via the self-manage suite)
	// are already hidden — so hiding the reads keeps the secret/config family
	// consistent instead of split across the name-only and hidden tiers.
	//   - read/write/list_config : the agent editing its OWN prompts/instructions
	//   - secret_list/secret_get  : vault reads, only on credential-backed tasks
	// MarkHidden on a name not built for this agent is a harmless no-op.
	reg.MarkHidden(
		"read_config", "write_config", "list_config",
		"secret_list", "secret_get",
	)
	// Role-aware eager trim: a read-only agent can never have a write approved, so
	// shipping the mutating tools' schemas every turn is pure waste. Demote them to
	// load-on-demand for read-only agents (still reachable via activate_tools, and
	// still execution-gated by the permission layer). "ask"/"auto" keep them eager.
	if agent.PermissionMode == "read-only" {
		reg.MarkLazy("Write", "Edit") // write_config already lazy above
	}
	// Per-tool visibility overrides from the workspace tools screen. HiddenTools
	// (the "NameOnly" chip) demotes a tool to load-on-demand AND renders it as
	// name-only in the catalog (Claude Code deferred-tool style — name shown,
	// schema pulled via activate_tools/tool_search). ShownTools is applied last
	// (see below) to force a default-lazy/hidden tool — e.g. the self-management
	// suite — back into the every-turn context. Loaded once for both passes.
	wsToolCfg, _ := r.db.GetWorkspaceToolConfig(ctx)
	if len(wsToolCfg.HiddenTools) > 0 {
		reg.MarkNameOnly(wsToolCfg.HiddenTools...)
	}

	if servers, err := r.db.ListEnabledMCPServers(ctx); err != nil {
		r.logger.Warn("list mcp servers failed", "error", err)
	} else if len(servers) > 0 && r.mcpPool != nil {
		// Persistent-pool build: each enabled server is reached over a live session
		// reused across turns (no gateway session churn), and a server's session
		// state (e.g. the gateway's activate_tools) survives across calls. The
		// catalog refreshes on tools/list_changed (and a safety-net TTL).
		cfgs := make([]mcp.ServerConfig, 0, len(servers))
		for _, m := range servers {
			cfgs = append(cfgs, toServerConfig(m))
		}
		entries, cfgByServer, errs := r.mcpPool.Catalog(ctx, cfgs)
		for name, e := range errs {
			r.logger.Warn("mcp catalog build failed", "server", name, "error", e)
		}
		caller := func(cctx context.Context, namespaced string, args json.RawMessage) (mcp.CallToolResult, error) {
			return r.mcpPool.Call(cctx, cfgByServer, namespaced, args)
		}
		reg.AttachMCP(entries, cfgByServer, caller)
	}

	// ShownTools override (applied LAST so it wins over every default + MCP lazy
	// mark): force these tools eager so they ride in the per-turn context. This is
	// how a user surfaces otherwise-hidden tools — notably the self-management
	// suite — via the tools screen's "Göster" toggle.
	if len(wsToolCfg.ShownTools) > 0 {
		reg.Unlazy(wsToolCfg.ShownTools...)
	}

	// Wire the lazy-loading meta-tools once the full lazy catalog (self-management
	// + MCP) is known. They are eager (always shipped) so the model can always
	// discover and activate on-demand tools. Skipped when nothing is lazy.
	if lazyCat := reg.LazyCatalog(nil); len(lazyCat) > 0 {
		active := activeToolsFromCtx(ctx) // nil for catalog/preview calls (no-op meta-tools)
		deact := tools.NewDeactivateToolsTool(active)
		// deactivate_tools is itself name-only on the native path (marked above), so it
		// is added AFTER LazyCatalog was snapshotted and would be missing from the
		// activate/search meta-tools' known set — making it impossible to activate. Add
		// its entry to the catalog those meta-tools see so it activates like any other
		// load-on-demand tool.
		if reg.IsLazy(deact.Def().Name) {
			d := deact.Def()
			lazyCat = append(lazyCat, providers.ToolDef{Name: d.Name, Description: d.Description})
		}
		reg.Add(
			tools.NewActivateToolsTool(active, lazyCat),
			deact,
			tools.NewToolSearchTool(lazyCat),
		)
	}
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
// only when it is active at the workspace level AND not on the agent's denylist
// AND permitted by the agent's allowlist. The denylist is the user-facing model
// (empty = all tools); the allowlist is the legacy subagent-profile restriction
// (empty = all). Both empty + no workspace denylist => nil (offer everything).
func (r *Runtime) toolFilter(ctx context.Context, agent db.Agent) func(string) bool {
	disabled := r.workspaceDisabledSet(ctx)
	agentAllow := allowFunc(agent) // nil => agent allows all
	agentBlock := blockFunc(agent) // nil => agent blocks nothing
	if disabled == nil && agentAllow == nil && agentBlock == nil {
		return nil
	}
	return func(name string) bool {
		if disabled[name] {
			return false
		}
		if agentBlock != nil && agentBlock(name) {
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

// ShippedToolCatalog returns the tools whose full schemas are actually sent at
// the START of a turn: the agent's eager (non-lazy) tools. Lazy tools are not
// here — they live in the load-on-demand catalog block and are pulled via
// activate_tools. Used by the context preview for an honest token split.
func (r *Runtime) ShippedToolCatalog(ctx context.Context, agent db.Agent) []providers.ToolDef {
	return r.buildRegistry(ctx, agent).ActiveDefs(r.toolFilter(ctx, agent), nil)
}

// LazyToolCatalog returns name+description for every lazy (on-demand) tool the
// agent may activate: self-management + MCP, minus its denylist. Their schemas
// are NOT shipped at turn start; they live in the system prompt catalog block.
func (r *Runtime) LazyToolCatalog(ctx context.Context, agent db.Agent) []providers.ToolDef {
	return r.buildRegistry(ctx, agent).LazyCatalog(r.toolFilter(ctx, agent))
}

// LazyToolsCatalogBlock renders the "Available Tools (load on demand)" system-
// prompt section for an agent: the name + summary of every lazy tool it may
// activate (self-management + MCP, minus its denylist). Returns "" when none.
// Part of the cached static prefix (stable per agent/workspace tool config).
func (r *Runtime) LazyToolsCatalogBlock(ctx context.Context, agent db.Agent) string {
	reg := r.buildRegistry(ctx, agent)
	filter := r.toolFilter(ctx, agent)
	// A claude-cli agent reaches these tools as MCP tools (namespaced) via the CLI's
	// own ToolSearch — NOT SwarmGo's native activate_tools. Render the block in CLI
	// form for it (mirrors how the skills block uses skillToolNameFor). The empty
	// provider is the keyless claude-cli default; native API providers take false.
	cli := agent.Provider == "" || agent.Provider == "claude-cli"
	// Visible lazy tools are enumerated; the hidden self-management suite is folded
	// into a single skill pointer (rendered when hiddenCount > 0).
	return renderLazyToolCatalog(reg.VisibleLazyCatalog(filter), reg.HiddenLazyCount(filter), cli)
}

// lazyCatalogMCPListLimit caps how many MCP (namespaced) lazy tools are listed
// individually in the load-on-demand catalog block. Above it, MCP tools are
// summarised per server with a count and the model is pointed at tool_search —
// this keeps the cached system-prompt prefix lean in MCP-heavy workspaces, where
// a single server can expose hundreds of tools. Built-in lazy tools (the
// self-management family) are always listed in full: they are few and high-value.
// This is the SwarmGo analogue of the Anthropic "tool search" pattern — search
// instead of enumerate once the catalog grows large.
const lazyCatalogMCPListLimit = 30

// cliLazyBridgeExcluded names built-in lazy tools NOT advertised to the claude-cli
// Interaction MCP bridge, so the CLI-form catalog must not list them (the CLI uses
// its OWN equivalent). Mirrors tools.bridgeExcluded — keep in sync. run_subagent is
// eager (never in the lazy catalog), so WebFetch is the only one that surfaces here.
var cliLazyBridgeExcluded = map[string]bool{
	"WebFetch":     true, // CLI has its own native WebFetch
	"run_subagent": true, // bridged explicitly via interactionToolSpecs, not the lazy path
	// deactivate_tools is a SwarmGo-native meta-tool (paired with activate_tools);
	// the CLI uses its OWN ToolSearch, so this is never bridged — keep it out of the
	// CLI catalog even though it is name-only on the native path.
	"deactivate_tools": true,
}

// catalogDisplayName maps a registry tool name to the identifier the target agent
// must actually call. Native agents call the bare/registry name as-is. A claude-cli
// agent reaches everything as MCP tools, so the name is namespaced: a namespaced
// MCP tool (server__tool) gains the CLI's "mcp__" prefix, and a built-in gains the
// Interaction MCP prefix. Returns ok=false for built-ins the CLI does not bridge
// (it has its own), so the caller skips them.
func catalogDisplayName(name string, cli bool) (string, bool) {
	if !cli {
		return name, true
	}
	if _, _, ok := mcp.SplitNamespaced(name); ok {
		return "mcp__" + name, true // server__tool → mcp__server__tool
	}
	if cliLazyBridgeExcluded[name] {
		return "", false // not bridged to the CLI (CLI-native)
	}
	return interactionToolPrefix + name, true // built-in via the Interaction MCP bridge
}

// renderLazyToolCatalog builds the load-on-demand tool catalog block from the
// VISIBLE lazy tool defs (name + description). Built-in lazy tools are always
// listed; namespaced MCP tools are listed individually only while under
// lazyCatalogMCPListLimit, otherwise summarised per server. hiddenCount > 0 appends
// a single pointer to the `swarmgo-self-management` skill in place of enumerating
// the hidden suite. Returns "" when there is nothing to show.
//
// cli renders the block for a claude-cli agent, which reaches these tools as MCP
// tools: names are namespaced (mcp__swarmgo_interaction__<name> for built-ins,
// mcp__<server>__<tool> for MCP) and loaded via the CLI's own ToolSearch — NOT
// SwarmGo's native activate_tools (the CLI has neither activate_tools nor
// tool_search). CLI-native built-ins (WebFetch) are dropped. This mirrors how the
// skills block (CatalogBlockForAgentTool) already adapts to the CLI.
func renderLazyToolCatalog(lazy []providers.ToolDef, hiddenCount int, cli bool) string {
	if len(lazy) == 0 && hiddenCount == 0 {
		return ""
	}
	// Separate built-in lazy tools from namespaced MCP tools (server__tool).
	var builtin, mcpTools []providers.ToolDef
	for _, d := range lazy {
		if _, _, ok := mcp.SplitNamespaced(d.Name); ok {
			mcpTools = append(mcpTools, d)
		} else {
			builtin = append(builtin, d)
		}
	}

	var b strings.Builder
	b.WriteString("# Available Tools (load on demand)\n")
	if cli {
		b.WriteString("These SwarmGo tools are exposed as MCP tools and may be DEFERRED (schema not " +
			"preloaded). They are listed by their EXACT namespaced names below (some with a short summary). " +
			"Before your first call, load one with `ToolSearch` (e.g. `select:<name>`); calling an unloaded " +
			"name returns \"No such tool available\". Load everything you expect to need in one call.\n")
	} else {
		b.WriteString("These tools are NOT loaded yet — only their names (and a short summary for some) are shown. " +
			"Entries listed by name alone are deferred: use `tool_search` to discover what they do by keyword. " +
			"To use any tool, first call `activate_tools` with its exact name(s); its full schema becomes " +
			"available on your next step. Activate everything you expect to need for a task in one call.\n")
	}
	for _, d := range builtin {
		if name, ok := catalogDisplayName(d.Name, cli); ok {
			writeLazyToolLine(&b, name, d.Description)
		}
	}

	switch {
	case len(mcpTools) == 0:
		// nothing more to add
	case len(mcpTools) <= lazyCatalogMCPListLimit:
		for _, d := range mcpTools {
			if name, ok := catalogDisplayName(d.Name, cli); ok {
				writeLazyToolLine(&b, name, d.Description)
			}
		}
	default:
		// Too many MCP tools to enumerate without bloating the cached prefix:
		// summarise per server and defer individual discovery to search.
		counts := map[string]int{}
		var order []string
		for _, d := range mcpTools {
			srv, _, _ := mcp.SplitNamespaced(d.Name)
			if _, seen := counts[srv]; !seen {
				order = append(order, srv)
			}
			counts[srv]++
		}
		sort.Strings(order)
		findHint, actHint := "tool_search(\"keyword\")", "activate_tools"
		if cli {
			findHint, actHint = "ToolSearch", "load"
		}
		fmt.Fprintf(&b, "\n%d more tools are available from MCP servers but not listed individually "+
			"(to save context). Find one with `%s`, then `%s` it. Servers:\n", len(mcpTools), findHint, actHint)
		for _, srv := range order {
			label := srv
			if cli {
				label = "mcp__" + srv
			}
			fmt.Fprintf(&b, "- `%s` — %d tools\n", label, counts[srv])
		}
	}

	// Self-management suite: kept out of the per-turn enumeration to save context.
	// Point the model at the skill (which documents the full catalog + how to load
	// them) and at the search tool as the quick path. The skill + search names are
	// namespaced for the CLI, bare for native.
	if hiddenCount > 0 {
		skillTool, findHint, actHint := "use_skill", "tool_search(\"keyword\")", "activate_tools"
		if cli {
			skillTool = interactionToolPrefix + "use_skill"
			findHint, actHint = "ToolSearch", "load"
		}
		fmt.Fprintf(&b, "\n%d self-management tools (manage agents, flows, schedules, tasks, hooks, "+
			"MCP servers, skills, workspaces, app settings, your own prompts/config, secrets, memory, logs) are available but "+
			"not listed here to save context. Load the `swarmgo-self-management` skill (via `%s`) "+
			"for the full catalog and how to use them, or find one directly with `%s` "+
			"— then `%s` the names you need.\n", hiddenCount, skillTool, findHint, actHint)
	}
	return strings.TrimSpace(b.String())
}

// writeLazyToolLine renders one load-on-demand catalog entry. A tool with a
// summary gets "- `name` — summary"; a NameOnly tool (summary suppressed in
// VisibleLazyCatalog) gets just "- `name`" — the Claude Code deferred-tool style,
// where the model sees the name and discovers the rest via search. name is the
// already-resolved display identifier (namespaced for the CLI path).
func writeLazyToolLine(b *strings.Builder, name, description string) {
	if strings.TrimSpace(description) == "" {
		fmt.Fprintf(b, "- `%s`\n", name)
		return
	}
	fmt.Fprintf(b, "- `%s` — %s\n", name, description)
}

// WorkspaceToolCatalog returns the full, unfiltered tool catalog (every built-in
// plus every enabled MCP server's tools) for the workspace tools screen, where
// each tool's active/inactive state is toggled independently of any agent.
func (r *Runtime) WorkspaceToolCatalog(ctx context.Context) []providers.ToolDef {
	return r.buildRegistry(ctx, db.Agent{}).Defs(nil)
}

// WorkspaceToolCatalogWithState is WorkspaceToolCatalog plus, for each tool, its
// effective load-on-demand state after all marks are applied — code defaults
// (self-management, MCP, etc.) AND the workspace HiddenTools/ShownTools overrides.
// It returns two sets so the tools screen can distinguish the tiers:
//   - lazy:   any load-on-demand tool (not shipped every turn). Drives the
//     "NameOnly" chip for name-only / MCP tools.
//   - hidden: the subset folded OUT of the per-turn catalog into the
//     self-management skill pointer (not enumerated by name). Drives the distinct
//     "Self-mgmt" chip. hidden ⊆ lazy.
func (r *Runtime) WorkspaceToolCatalogWithState(ctx context.Context) ([]providers.ToolDef, map[string]bool, map[string]bool) {
	reg := r.buildRegistry(ctx, db.Agent{})
	defs := reg.Defs(nil)
	lazy := make(map[string]bool, len(defs))
	hidden := make(map[string]bool, len(defs))
	for _, d := range defs {
		if reg.IsLazy(d.Name) {
			lazy[d.Name] = true
		}
		if reg.IsHidden(d.Name) {
			hidden[d.Name] = true
		}
	}
	return defs, lazy, hidden
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

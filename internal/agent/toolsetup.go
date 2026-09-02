package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/codemode"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/mcp"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/tools"
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
// ToServerConfig converts a stored MCP server row into an mcp.ServerConfig. Exported
// so the external gateway (Doc 52 Faz 3) can expose the same enabled servers the agent
// tool loop connects to, via one converter (no second parser).
func ToServerConfig(m db.MCPServer) mcp.ServerConfig { return toServerConfig(m) }

func toServerConfig(m db.MCPServer) mcp.ServerConfig {
	var args []string
	_ = json.Unmarshal([]byte(m.Args), &args)
	env := map[string]string{}
	_ = json.Unmarshal([]byte(m.EnvConfig), &env)
	headers := map[string]string{}
	_ = json.Unmarshal([]byte(m.HeadersConfig), &headers)
	return mcp.ServerConfig{
		Name:        m.Name,
		Transport:   m.Transport,
		Command:     m.Command,
		Args:        args,
		URL:         m.URL,
		Env:         env,
		Headers:     headers,
		Description: m.Description,
	}
}

// patternPredicate compiles a list of tool-name patterns into a matcher. A
// "group:<category>" key matches every built-in tool in that functional
// category; a pattern ending in "*" matches by prefix; otherwise it matches
// exactly. An empty list yields a nil predicate (caller treats nil as "no
// constraint").
func patternPredicate(patterns []string) func(string) bool {
	if len(patterns) == 0 {
		return nil
	}
	return func(name string) bool {
		for _, p := range patterns {
			if tools.IsGroupKey(p) {
				if tools.MatchesGroup(name, p) {
					return true
				}
			} else if strings.HasSuffix(p, "*") {
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
//
// A malformed allowlist is an ERROR, and the predicate returned with it denies
// EVERY tool. The unmarshal error used to be discarded, which left patterns nil —
// and a nil pattern list is "no constraint", i.e. every tool allowed. A permission
// document that cannot be read must never widen the permission.
func allowFunc(agent db.Agent) (func(string) bool, error) {
	raw := strings.TrimSpace(agent.AllowedTools)
	if raw == "" {
		return nil, nil
	}
	var patterns []string
	if err := json.Unmarshal([]byte(raw), &patterns); err != nil {
		return func(string) bool { return false }, fmt.Errorf("allowed_tools: %w", err)
	}
	return patternPredicate(patterns), nil
}

// blockFunc builds a tool-name predicate reporting whether a tool is BLOCKED for
// this agent. The denylist is the "blocked"-tier slice of the agent's tool
// override map (which folds in the legacy BlockedTools list), so the unified
// 5-tier model and the old standalone denylist resolve to the same predicate.
// Nothing blocked => nil predicate (caller treats nil as "no constraint").
//
// An EXACT non-blocked override beats a broad blocked key: banning
// "group:files" while pinning "Read" to a visibility tier keeps Read usable.
// The exemption is deliberately limited to exact names — the same specificity
// rule applyVisibilityOverrides enforces.
//
// A malformed override document is an ERROR, and the predicate returned with it
// reports every tool BLOCKED: an unreadable denylist must not degrade into
// "nothing is blocked".
func blockFunc(agent db.Agent) (func(string) bool, error) {
	overrides, err := ParseToolOverridesErr(agent)
	if err != nil {
		return func(string) bool { return true }, err
	}
	pred := patternPredicate(blockedPatterns(overrides))
	if pred == nil {
		return nil, nil
	}
	exempt := map[string]bool{}
	for key, tier := range overrides {
		if tier != TierBlocked && !tools.IsGroupKey(key) && !strings.HasSuffix(key, "*") {
			exempt[key] = true
		}
	}
	if len(exempt) == 0 {
		return pred, nil
	}
	return func(name string) bool {
		if exempt[name] {
			return false
		}
		return pred(name)
	}, nil
}

// readTrackerFor returns the freshness read-tracker for a session, creating it on
// first use. An empty session id (catalog/preview builds with no session on ctx)
// yields nil, which disables the Edit/Write guard for that build.
func (r *Runtime) readTrackerFor(sessionID string) *tools.ReadTracker {
	if sessionID == "" {
		return nil
	}
	v, _ := r.readTrackers.LoadOrStore(sessionID, tools.NewReadTracker())
	return v.(*tools.ReadTracker)
}

// shellMgrFor returns the background-shell manager for a session, creating it on
// first use. An empty session id (catalog/preview builds with no session on ctx)
// yields nil, which disables run_in_background for that build.
func (r *Runtime) shellMgrFor(sessionID string) *tools.ShellManager {
	if sessionID == "" {
		return nil
	}
	v, _ := r.shellMgrs.LoadOrStore(sessionID, tools.NewShellManager())
	return v.(*tools.ShellManager)
}

// buildRegistry assembles the tool registry for an agent: built-in tools plus
// the catalog of every enabled MCP server in the workspace. cfgByServer maps
// sanitized server names back to their configs for dispatch.
func (r *Runtime) buildRegistry(ctx context.Context, agent db.Agent) *tools.Registry {
	builtins := []tools.Tool{
		tools.NewWebFetchTool(),
		// WebSearch: native web search for every NATIVE-API provider (anthropic,
		// minimax, openrouter). Registered unconditionally — exactly like
		// its sibling WebFetch — so it also appears in the workspace tools catalog.
		// claude-cli never receives it: it has its OWN native WebSearch/WebFetch, so
		// TionHarness's are withheld to avoid doubling the surface. codex-cli DOES
		// receive both over the Interaction bridge (BridgeTools): codex ships no
		// native WebFetch at all and its own web_search is OFF by default, so without
		// the bridge a codex agent has no web access and starts inventing sources.
		// Backed by the workspace vault (a self-hosted SEARXNG_URL or a TAVILY_API_KEY).
		tools.NewWebSearchTool(r.vault),
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
		// update_session: mutate THIS session's metadata in one call — title,
		// working dir, tags, archive. Replaces the
		// former per-field tools (set_session_title/_working_dir/archive_session/
		// set_session_tags). No-op without a session
		// sink (i.e. outside a session-bound turn). Tags shared with the UI also
		// enrol the session into tag-triggered automations.
		tools.NewUpdateSessionTool(),
		// mermaid_validate: lint a Mermaid diagram (recognised type + balanced
		// brackets/quotes) before emitting it. Pure, read-only, no deps.
		tools.NewMermaidValidateTool(),
		// render_template: fill a branded HTML template (Go html/template) with JSON
		// data, write it under the session render dir, and return only the path +
		// warnings (never the HTML) for inline html-preview. Session-scoped: an empty
		// render dir (catalog/preview build with no session) makes the tool fail loudly
		// when called. No arbitrary code execution, so it is NOT behind the shell gate.
		tools.NewRenderTemplateTool(r.SessionRenderDir(SessionIDFrom(ctx))),
	}

	// Skills: an agent may load its ASSIGNED skills plus every SHARED (on-demand)
	// skill. Their summaries are advertised in the system prompt; bodies stay on
	// disk until use_skill is called (lazy). The tool is restricted to that set —
	// a restricted skill is unreachable unless explicitly assigned.
	if r.skills != nil {
		// skill_validate: read-only check of a skill's SKILL.md (slug/frontmatter/
		// body). Always available when a skill store exists — pairs with create_skill/
		// update_skill so an agent can self-check authored skills, independent of which
		// skills are assigned.
		builtins = append(builtins, tools.NewSkillValidateTool(agentSkillWriter{store: r.skills}))
		if allow := r.skills.AllowedFor(agent.Skills); len(allow) > 0 {
			lib := agentSkillLib{store: r.skills, allow: allow}
			builtins = append(builtins, tools.NewUseSkillTool(lib))
			// SK-2: skill_search lets the agent discover on-demand/conditional skills
			// that are deliberately kept out of the per-turn catalog.
			builtins = append(builtins, tools.NewSkillSearchTool(lib))
		}
	}

	// Subagents: the generic run_subagent tool launches an isolated worker — a
	// built-in profile (explore/coder/reviewer) or an existing agent — and blocks
	// until it is done, gathering only its final result so a sub-task's tool output
	// never floods this turn's context. Always shipped (like self-management, 2026-07-01):
	// availability is managed per-tool from the Tools screen (denylist), not by an
	// app-settings master toggle; the runner still enforces depth/cycle/budget/
	// concurrency guards on every call.
	builtins = append(builtins, tools.NewRunSubagentTool())

	// Coordinator/worker tools (M2, _Docs/47). withCoordination installs the runner
	// on every session's turn, but populates only the capabilities that session
	// actually has, and each tool is registered from the presence of ITS function —
	// so the three surfaces stay independent:
	//   - the worker-driving tools: coordinator mode is on (root OR a mid-level node
	//     of a deep tree; the old "workers never get these" rule is what capped the
	//     tree at one level, and the depth/subtree budgets replaced it);
	//   - report_to_coordinator: this session has a coordinator ABOVE it;
	//   - set_coordinator_mode: always, otherwise a plain session could never turn
	//     the capability on for itself.
	if cf := tools.CoordinationFrom(ctx); cf != nil {
		if cf.Spawn != nil {
			builtins = append(builtins,
				tools.NewSpawnWorkerTool(),
				tools.NewSendToWorkerTool(),
				tools.NewStopWorkerTool(),
				tools.NewListWorkersTool(),
			)
		}
		if cf.Report != nil {
			builtins = append(builtins, tools.NewReportToCoordinatorTool())
		}
		if cf.SetMode != nil {
			builtins = append(builtins, tools.NewSetCoordinatorModeTool())
		}
		// trajectory (Rota, F1): a coordinator reads / declares its tree's plan.
		if cf.Trajectory != nil {
			builtins = append(builtins, tools.NewTrajectoryTool())
		}
	}

	// Cross-session awareness: the list_sessions pull tool (complements the pushed
	// context block). Always on, like the other pull tools.
	builtins = append(builtins, tools.NewListSessionsTool(r.db))
	// archive_sessions: bulk-archive OTHER sessions in THIS workspace (the
	// "manage siblings" complement to update_session, which only edits the
	// current one). Bound to r.db so it is physically workspace-scoped, and it
	// always excludes the current session (resolved here at build time) — so an
	// agent asked to "clean up old sessions" never has to fall back to raw REST
	// (which loses workspace scoping and once archived the wrong workspace).
	builtins = append(builtins, tools.NewArchiveSessionsTool(r.db, SessionIDFrom(ctx)))
	// conversation_search: full-text search across the workspace's message
	// history (deeper than list_sessions' titles+summaries).
	builtins = append(builtins, tools.NewConversationSearchTool(r.db))

	// read_session_debug: the agent reads its OWN session's structured debug
	// journal (turn timings, token spend, tool latency/errors, anomalies) to
	// self-diagnose and optimise. Always-on (read-only, the sibling of
	// conversation_search for self-improvement) whenever the debug journal is on,
	// so it works out of the box — including for the default keyless claude-cli
	// agent — rather than being buried in the hidden-lazy self-manage tier.
	if r.tun != nil && r.tun.DebugJournalEnabled() {
		builtins = append(builtins, tools.NewReadSessionDebugTool(r.db))
	}

	// get_view: the PULL channel of the projection layer — a compact, deterministic
	// summary of a large entity (today: flow runs) instead of reading its raw state.
	// Always-on and read-only; it is strictly cheaper than the get_flow_run +
	// parse-the-state-JSON path it replaces (_Docs/66).
	// WithSources is what keeps the agent's projections identical to the ones the
	// Explorer map renders: without it the skill / insight / logs nodes would
	// report their source as unavailable to the agent while the UI showed them.
	viewSources := tools.ViewSources{Skills: r.skills, Logs: r.logs}
	builtins = append(builtins, tools.NewGetViewTool(r.db).WithSources(r.wsName, viewSources))

	// expand: the structural drill-down companion to get_view (_Docs/68). Lists a
	// node's children (the Workspace Explorer map's edges) so an agent can fan out
	// over the workspace tree cheaply and get_view only the branch that matters.
	builtins = append(builtins, tools.NewExpandTool(r.db).WithSources(r.wsName, viewSources))

	// read_lessons / delete_lesson: the agent inspects and prunes the workspace's
	// auto-collected failure lessons (self-healing). The newest few already ride
	// its context; these tools expose the full set + ids. Gated by the same
	// setting that produces lessons — with the loop off there is nothing to read.
	if r.tun != nil && r.tun.LessonReflect() {
		builtins = append(builtins, tools.NewReadLessonsTool(r.db), tools.NewDeleteLessonTool(r.db))
	}

	// insight_scan / insight_list_findings: trigger a retrospective scan and review
	// what it surfaced (app-fix + workspace-opt findings). Always built so an agent
	// can self-improve out of the box; scans are incremental/idempotent (_Docs/60).
	// Default visibility is NAME-ONLY (see the MarkNameOnly block below).
	builtins = append(builtins, tools.NewInsightScanTool(r), tools.NewInsightFindingsTool(r.db), tools.NewInsightApplyFindingTool(r.db))

	// codebase_workspace_search: fan out codebase-memory's project-scoped search_code
	// across EVERY project in the server's store, for "where is X?" queries
	// (graph/architecture queries are already fleet-wide; this fills the text-search
	// gap). Only when an enabled codebase-memory server is present — the tool shells
	// out to that same executable.
	if cmd := r.codebaseMemoryCmd(ctx); cmd != "" {
		builtins = append(builtins, tools.NewCodebaseWorkspaceSearchTool(cmd))
	}

	// get_session_info: the read counterpart of the session-edit tools — the
	// agent inspects its own session's metadata (title/state/tags/cwd/role/
	// lineage) before mutating it, or orients itself in a fresh autonomous turn.
	builtins = append(builtins, tools.NewGetSessionInfoTool(r.db))

	// update_user_preferences: persist durable user facts (name/location/timezone/
	// preference notes) into the app-wide profile injected into every turn. A
	// narrow wrapper over the settings bridge, so only offered when it is wired.
	if r.settingsBridge != nil {
		builtins = append(builtins, tools.NewUpdateUserPreferencesTool(r.settingsBridge))
	}

	// Workspace secret vault: one tool (action=list/get/set/delete) to discover,
	// fetch, store and remove stored credentials (API keys, tokens, passwords) for
	// the tasks agents run. Replaces the former secret_list/get/set/delete quartet.
	if r.vault != nil {
		builtins = append(builtins, tools.NewSecretTool(r.vault))
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
	// Freshness guard: a session-scoped read-tracker lets Edit/Write detect a file
	// changed out-of-band since it was last read. Nil (guard off) for catalog/preview
	// builds without a session on ctx, or when the tunable is disabled — nil disables
	// enforcement in the fs tools.
	var readTracker *tools.ReadTracker
	if r.tun != nil && r.tun.FileFreshnessGuard() {
		readTracker = r.readTrackerFor(SessionIDFrom(ctx))
	}
	if sb.Ready() {
		builtins = append(builtins,
			tools.NewFSReadFileTool(sb, readTracker),
			tools.NewFSWriteFileTool(sb, readTracker),
			tools.NewFSEditFileTool(sb, readTracker),
			// apply_patch: multi-hunk / multi-file unified-diff editing (the batch
			// sibling of Edit), sharing the same freshness guard.
			tools.NewFSApplyPatchTool(sb, readTracker),
			tools.NewFSListDirTool(sb),
			tools.NewFSGlobTool(sb),
			tools.NewFSGrepTool(sb),
			// config_validate: well-formed-JSON + known-shape check for TionHarness config
			// files, rooted at this turn's working dir. Read-only.
			tools.NewConfigValidateTool(sb),
		)
		// The shell tools are high-risk; offer them only when explicitly enabled. Two
		// siblings share one execution core: Bash (POSIX — /bin/sh, or bash.exe on
		// Windows) and PowerShell (pwsh/powershell.exe). Each is registered only when
		// its backing shell is present, so the model gets a correctly-NAMED tool and
		// emits the matching syntax. On every OS at least one is available (Unix→Bash,
		// Windows→PowerShell). transform_data also runs arbitrary host code (python/
		// node/bun) under the same gate; it reshapes large data into a JSON file
		// (referenced as a table src) without inlining rows into context.
		if r.tun.ShellEnabled() {
			// Session-scoped background-shell manager: wired onto both shell tools so
			// run_in_background can launch detached processes, plus the three management
			// tools (shell_output/shell_kill/shell_list) that poll and reap them. Nil for
			// catalog/preview builds (no session) → background execution simply unavailable.
			shellMgr := r.shellMgrFor(SessionIDFrom(ctx))
			// Route foreground shell output through the token-optimizer (sqz) in-process
			// when wired — same filter the bridged (claude-cli) path uses, so native
			// providers save tokens identically. nil when sqz is not opted-in.
			shellFilter := r.sqzShellFilter(ctx)
			// Command-layer optimizer (rtk) at the other end of the same call: it
			// rewrites the command so it emits less, where sqz compresses what it
			// emitted. Independently gated; both, either or neither may be active.
			cmdFilter := r.rtkCommandFilter(ctx)
			if sh := tools.NewShellTool(sb); sh.Available() {
				builtins = append(builtins, sh.WithManager(shellMgr).WithOutputFilter(shellFilter).WithCommandFilter(cmdFilter))
			}
			if ps := tools.NewPowerShellTool(sb); ps.Available() {
				builtins = append(builtins, ps.WithManager(shellMgr).WithOutputFilter(shellFilter).WithCommandFilter(cmdFilter))
			}
			if shellMgr != nil {
				// One control tool (action=output/kill/list) polls and reaps the
				// detached processes started with run_in_background.
				builtins = append(builtins, tools.NewShellManageTool(shellMgr))
			}
			builtins = append(builtins, tools.NewTransformDataTool(sb))
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

	// Self-management suite: let an agent create/edit/delete agents, flows and
	// schedules, manage artifacts, add memories and read logs. Provenance is
	// enforced — agents only touch agent-created entities. Always built now (the
	// former `enableSelfManage` master toggle was removed): these tools default to
	// the HIDDEN visibility tier below (folded into the self-management skill
	// pointer, ~zero per-turn cost) and are promoted per-tool via the tools screen.
	// Self-management suite (agents/flows/schedules/tasks/hooks/mcp/skills/
	// settings/workspaces/memory/logs). Extracted to selfManageBuiltins so this
	// method stays a readable outline; the whole run is marked HIDDEN below.
	selfManageStart := len(builtins)
	builtins = append(builtins, r.selfManageBuiltins(agent)...)

	reg := tools.NewRegistry(builtins...)

	// Lazy tool loading: the self-management suite is large and used in a minority
	// of turns, so its schemas are loaded on demand (activate_tools) rather than
	// shipped every turn. MCP tools are marked lazy inside AttachMCP.
	//
	// HIDDEN: the self-management family is also kept OUT of the rendered
	// load-on-demand catalog block — dozens of name+summary lines would otherwise
	// ride in every turn's cached prefix. Their catalog + usage lives in the
	// `tionharness-self-management` skill (advertised in Available Skills); the block
	// shows a single pointer to it. They stay activatable (activate_tools) and
	// searchable (tool_search), so the skill is the documented path, not the only one.
	for _, t := range builtins[selfManageStart:] {
		reg.MarkHidden(t.Def().Name)
		// Stamp stable self-management membership so the claude-cli bridge keeps
		// advertising these even if a workspace override later promotes one to the
		// full tier (which clears the hidden/lazy marks). See BridgeableDefsFiltered.
		reg.MarkSelfManaged(t.Def().Name)
	}
	// Default per-tool tiers (name-only + hidden) live as DATA in
	// tools.DefaultTiers() — see internal/tools/tierdefaults.go for the table and
	// the rationale of each row (including which tools are deliberately kept eager).
	// Applied AFTER the self-manage hidden loop above, so the table's authoritative
	// entry for a promoted name (handoff_session, send_message → name-only) wins;
	// their MarkSelfManaged stamp survives, which the claude-cli bridge needs.
	defaults := tools.DefaultTiers()
	reg.ApplyToolDefaults(defaults)
	// Role-aware eager trim: a read-only agent can never have a write approved, so
	// shipping the mutating tools' schemas every turn is pure waste. Demote them to
	// load-on-demand for read-only agents (still reachable via activate_tools, and
	// still execution-gated by the permission layer). "ask"/"auto" keep them eager.
	if agent.PermissionMode == "read-only" {
		reg.MarkLazy("Write", "Edit") // write_config + apply_patch already lazy above
	}
	// Per-tool visibility overrides from the workspace tools screen are applied
	// AFTER AttachMCP below (so they win over both code defaults and the MCP
	// name-only default). Loaded here once.
	wsToolCfg, _ := r.db.GetWorkspaceToolConfig(ctx)

	// MCP catalog (may stay empty when there are no enabled servers): populated in
	// the pool branch below and reused by run_code's bindings.
	var mcpEntries []mcp.CatalogEntry
	var mcpCaller tools.MCPCaller

	if servers, err := r.db.ListEnabledMCPServers(ctx); err != nil {
		r.logger.Warn("list mcp servers failed", "error", err)
	} else if len(servers) > 0 && r.mcpPool != nil {
		// Persistent-pool build: each enabled server is reached over a live session
		// reused across turns (no gateway session churn), and a server's session
		// state (e.g. the gateway's activate_tools) survives across calls. The
		// catalog refreshes on tools/list_changed (and a safety-net TTL).
		cfgs := make([]mcp.ServerConfig, 0, len(servers))
		// Hybrid MCP scoping: a server marked scope="scoped" gets its OWN live
		// connection per (session, agent) instead of the shared workspace-wide one,
		// so its per-connection server state can't bleed between sessions and its
		// blast radius on hang/crash is one caller. The transport is still pooled
		// (idle-evicted, not spawned per turn), so shared servers are unaffected.
		// No session (catalog/preview build) → fall back to shared. Detail: _Docs/52.
		mcpScopeKey := ""
		if sid := SessionIDFrom(ctx); sid != "" {
			mcpScopeKey = sid + "|" + agent.ID
		}
		for _, m := range servers {
			cfg := toServerConfig(m)
			if m.Scope == "scoped" && mcpScopeKey != "" {
				cfg.ScopeKey = mcpScopeKey
			}
			// A file-writing MCP (Playwright) is denied writes outside its allowed
			// roots; add THIS turn's session scratchpad as a root so screenshot/PDF
			// saves land where the agent is told to write. Self-contained in
			// mcp_playwright.go (applyMCPScratchpadRoot); no-op for other servers.
			cfg = r.applyMCPScratchpadRoot(ctx, cfg, m, mcpScopeKey)
			cfgs = append(cfgs, cfg)
		}
		// A failed server silently loses ALL of its tools for the turn, so the log
		// line is not enough: hand the failures to the turn's collector (when one is
		// wired) so the tool loop can card them once. See mcpnotice.go.
		failures := mcpFailuresFrom(ctx)
		// Circuit breaker (mcpescalate.go): a server that failed the last
		// mcpFailStreakThreshold builds in a row is not dialed again until its
		// cooldown passes. It is carded as skipped, exactly like a failed one, and
		// probed once per cooldown. Without this a hung server stalled EVERY
		// registry build — every turn, the tools panel, the context preview — for
		// as long as the caller's context lived (2026-09-02, codebase-memory-mcp).
		live := make([]mcp.ServerConfig, 0, len(cfgs))
		for _, cfg := range cfgs {
			if isOpen, retryIn, streak := r.mcpFailStreaks.open(cfg.Name); isOpen {
				retry := retryIn.Round(time.Second)
				failures.record(cfg.Name, fmt.Sprintf("skipped after %d consecutive failures; next probe in %s", streak, retry))
				r.logger.Debug("mcp catalog: breaker open, server skipped", "server", cfg.Name, "consecutive", streak, "retry_in", retry)
				continue
			}
			live = append(live, cfg)
		}
		entries, cfgByServer, errs := r.mcpPool.Catalog(ctx, live)
		for name, e := range errs {
			// Escalate a STANDING outage exactly once (mcpescalate.go): repeating the
			// same WARN forever made a workspace where no turn could start look normal.
			streak := r.mcpFailStreaks.note(name)
			switch {
			case streak == mcpFailStreakThreshold:
				r.logger.Error("mcp catalog build failing repeatedly; this server's tools are unavailable",
					"server", name, "consecutive", streak, "error", e)
			default:
				r.logger.Warn("mcp catalog build failed", "server", name, "consecutive", streak, "error", e)
			}
			failures.record(name, e)
		}
		// Reset the streak for every server that catalogued fine this turn, so the
		// threshold measures the current outage rather than a lifetime total. Only
		// the servers actually dialed count: a skipped one must keep its streak.
		for _, cfg := range live {
			if _, bad := errs[cfg.Name]; !bad {
				r.mcpFailStreaks.clear(cfg.Name)
			}
		}
		caller := func(cctx context.Context, namespaced string, args json.RawMessage) (mcp.CallToolResult, error) {
			return r.mcpPool.Call(cctx, cfgByServer, namespaced, args)
		}
		reg.AttachMCP(entries, cfgByServer, caller)
		// Bundle-level defaults need the MCP entries to exist, so they run here
		// rather than next to ApplyToolDefaults. No-op for built-ins today (no
		// group rows ship), and workspace/agent overrides below still win.
		reg.ApplyBundleDefaults(defaults)
		mcpEntries, mcpCaller = entries, caller
	}

	// Code-execution mode (POC, _Docs/44): expose the MCP catalog AND TionHarness's own
	// eligible built-in tools as generated Python bindings behind a single run_code
	// tool, so tool schemas stay OUT of the context window and intermediate data
	// stays in the execution environment (the model orchestrates list→filter→act in
	// ONE call). Gated by CodeMode+Shell toggles + a working dir; the bridge re-
	// applies this agent's tool filter (plus the code-mode eligibility filter), so
	// code mode grants no tool the agent could not call directly. Available even
	// WITHOUT MCP servers now — the built-ins alone are enough. Native path only
	// (the CLI bridge excludes run_code, bridgeExcluded).
	if r.tun.CodeModeEnabled() && r.tun.ShellEnabled() && sb.Ready() {
		toolFilter := r.toolFilter(ctx, agent)
		// codeAllow = code-mode eligibility AND the agent's own filter. Eligibility is
		// keyed on bare built-in names; a namespaced MCP name is always eligible (never
		// in the exclude set), so this is safe for both binding kinds.
		codeAllow := func(name string) bool {
			return tools.CodeModeEligible(name) && (toolFilter == nil || toolFilter(name))
		}
		// Built-in tool defs to expose as the `tionharness` module (eligible + permitted).
		// run_code and the meta-tools are not in reg yet, so they can't self-expose.
		biDefs := reg.BuiltinDefs(codeAllow)
		biBindings := make([]codemode.BuiltinDef, 0, len(biDefs))
		for _, d := range biDefs {
			biBindings = append(biBindings, codemode.BuiltinDef{Name: d.Name, Description: d.Description, InputSchema: d.InputSchema})
		}
		var callBI tools.MCPCaller
		if len(biBindings) > 0 {
			// Dispatch a built-in through the SAME registry the native tool loop uses,
			// on the TURN ctx (carried in by run_code's Call), so sink/session-scoped
			// built-ins behave identically to a direct call.
			callBI = func(cctx context.Context, name string, args json.RawMessage) (mcp.CallToolResult, error) {
				res := reg.Call(cctx, providers.ToolCall{Name: name, Input: args})
				return mcp.CallToolResult{Text: res.Content, IsError: res.IsError}, nil
			}
		}
		if len(mcpEntries) > 0 || len(biBindings) > 0 {
			// Faz 2 hooks. Gate: the EXACT decision the native tool loop makes for a
			// direct call of the same tool in the same turn — permGate with this
			// agent's mode, the ctx-carried prompter/grants, and the audit logger. So
			// "ask" prompts per in-script mutation (with standing grants honoured),
			// "read-only" blocks (defense-in-depth: run_code itself is already
			// blocked there), autonomous ask-turns without a prompter deny — full
			// parity, no separate policy to maintain.
			gate := func(cctx context.Context, tool string, args json.RawMessage) (bool, string) {
				return permGate(withPermLogger(cctx, r.logger), agent.PermissionMode,
					providers.ToolCall{Name: tool, Input: args})
			}
			// Observer: one debug-journal tool event per in-script call, so the
			// per-message debug panel lists them exactly like native tool calls —
			// the observability that folding N calls into one run_code card loses.
			// The sub-step sink on the call ctx feeds the UI: each in-script call
			// becomes a nested trace row, and the tool loop's generic promotion turns
			// the run_code card into a collapsible StepSubagent — the same rendering
			// run_subagent gets. The mutex guards against a multi-threaded script
			// issuing concurrent bridge calls (each handler runs on its own goroutine).
			var obMu sync.Mutex
			observe := func(cctx context.Context, ob codemode.CallObservation) {
				detail := "via run_code"
				if ob.Denied {
					detail = "permission denied (via run_code)"
				}
				r.emitDebug(cctx, db.DebugEvent{
					Type:     db.DebugTool,
					AgentID:  agent.ID,
					Name:     ob.Tool,
					DurMs:    ob.DurMs,
					OutBytes: ob.OutBytes,
					Err:      ob.IsError,
					Error:    debugSummary(ob.Error, 500),
					Args:     debugToolArgs(ob.Args, ob.IsError),
					Detail:   detail,
				})
				if sink := subStepSinkFrom(cctx); sink != nil {
					st := TurnStep{
						Kind:    StepTool,
						Tool:    ob.Tool,
						Input:   capStepInput(ob.Args),
						IsError: ob.IsError,
						Output:  fmt.Sprintf("%s in %dms (result stays in the script)", humanStepBytes(ob.OutBytes), ob.DurMs),
					}
					if ob.Denied {
						st.Output = "permission denied"
						st.Reason = "permission_denied"
					}
					obMu.Lock()
					sink.steps = append(sink.steps, st)
					obMu.Unlock()
				}
			}
			reg.Add(tools.NewRunCodeTool(sb, mcpEntries, mcpCaller, biBindings, callBI, codeAllow, gate, observe))
		}
	}

	// Per-tool visibility overrides, applied LAST as a two-layer chain so each wins
	// over every code default + the MCP name-only default:
	//
	//	code default  <  workspace ToolVisibility  <  agent ToolOverrides
	//
	// Both force a tool into one of the four tiers — full / summary / name-only /
	// hidden. An unknown tier value is ignored; an unknown name is a harmless no-op.
	// The agent map's fifth tier ("blocked") is NOT a visibility state and is
	// deliberately skipped here: it is enforced one layer up, in toolFilter, which
	// removes the tool from the catalog outright.
	for name, tier := range wsToolCfg.ToolVisibility {
		reg.SetVisibility(name, tier)
	}
	if agentOv := visibilityOverrides(ParseToolOverrides(agent)); len(agentOv) > 0 {
		// Catalog names are needed only to expand "prefix*" override keys, which
		// SetVisibility (one exact name) cannot take directly.
		defs := reg.Defs(nil)
		names := make([]string, 0, len(defs))
		for _, d := range defs {
			names = append(names, d.Name)
		}
		applyVisibilityOverrides(reg, agentOv, names)
	}

	// Wire the lazy-loading meta-tools once the lazy catalog (self-management +
	// MCP) is known. They are eager (always shipped) so the model can always
	// discover and activate on-demand tools. Skipped when nothing is lazy.
	//
	// Every view handed to those meta-tools is built with THIS agent's tool filter
	// so a tool the agent may not use is neither listed nor activatable: the
	// catalog block already renders filtered, and an unfiltered known set let
	// activate_tools / tool_search resurrect a blocked tool by name.
	filter := r.toolFilter(ctx, agent)
	if lazyCat := reg.LazyCatalog(filter); len(lazyCat) > 0 {
		active := activeToolsFromCtx(ctx) // nil for catalog/preview calls (no-op meta-tools)
		deact := tools.NewDeactivateToolsTool(active)
		// deactivate_tools is itself name-only on the native path (marked above), so it
		// is added AFTER LazyCatalog was snapshotted and would be missing from the
		// activate/search meta-tools' known set — making it impossible to activate. Add
		// its entry to the catalog those meta-tools see so it activates like any other
		// load-on-demand tool.
		if d := deact.Def(); reg.IsLazy(d.Name) && (filter == nil || filter(d.Name)) {
			lazyCat = append(lazyCat, providers.ToolDef{Name: d.Name, Description: d.Description})
		}
		// eagerNames: the always-on built-ins (e.g. todo_write) so activate_tools can
		// answer a stray "activate an already-shipped tool" request with a clear
		// "already available" note instead of a misleading "unknown name".
		eagerNames := reg.EagerNames(filter)
		// Bundle index for activate_tools' group form: a blocked built-in never
		// surfaces as a name+summary line when the agent opens its category. The tool
		// additionally drops any member missing from the lazy catalog, so an eager
		// tool still cannot be "activated".
		bundles := reg.BundleIndex(filter)
		reg.Add(
			tools.NewActivateToolsToolBundled(active, lazyCat, eagerNames, bundles),
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
//
// The COORDINATION tools are exempt from the ALLOWLIST (only — the workspace
// switch and the explicit denylist still apply). The allowlist describes which
// WORK tools a profile persona may use; whether a session may drive workers or
// owes a report upward is a property of the SESSION, gated precisely by
// CoordinationFuncs, and the two must not be conflated.
//
// Found live: a coordinator spawning a profile target as a sub-coordinator
// (spawn_worker with a profile + coordinator:true) produced a session with
// coordinator mode ON and the coordinator manual in its prompt, while the
// profile's read-only allowlist stripped every coordination tool — including
// report_to_coordinator. It could neither delegate nor report, so it silently did
// the work itself (CLI-native Read/Edit/Bash do not pass through this filter, so
// nothing visibly failed). That is exactly the "asked for delegation, got a worker
// that cannot delegate and never says so" failure SpawnWorker refuses elsewhere.
func (r *Runtime) toolFilter(ctx context.Context, agent db.Agent) func(string) bool {
	disabled := r.workspaceDisabledSet(ctx)
	agentAllow, allowErr := allowFunc(agent) // nil => agent allows all
	agentBlock, blockErr := blockFunc(agent) // nil => agent blocks nothing
	// A permission document that cannot be parsed leaves the agent's real limits
	// UNKNOWN, so the only safe answer is "no tool at all" — loudly, on both the
	// server log and the session's debug journal. Degrading to the permissive
	// default is how a malformed allowlist used to hand an agent every tool.
	if err := errors.Join(allowErr, blockErr); err != nil {
		r.logger.Error("agent tool permission config is malformed; denying every tool",
			"agent", agent.ID, "error", err)
		r.emitDebug(ctx, db.DebugEvent{
			Type:    db.DebugError,
			AgentID: agent.ID,
			Name:    "tool_permission_config_malformed",
			Detail:  "agent tool permission config is malformed; every tool denied",
			Error:   err.Error(),
			Err:     true,
		})
		return func(string) bool { return false }
	}
	if disabled == nil && agentAllow == nil && agentBlock == nil {
		return nil
	}
	// Resolved once per filter build, not per name: it reads the MCP server list.
	// "" when the workspace has no enabled codebase-memory server (no exemption).
	exemptServer := r.allowlistExemptServer(ctx)
	return func(name string) bool {
		if disabled[name] {
			return false
		}
		if agentBlock != nil && agentBlock(name) {
			return false
		}
		if tools.IsCoordinationTool(name) {
			return true // session-gated, not allowlist-gated (see above)
		}
		if tools.IsSkillTool(name) {
			// Same class of prompt/permission mismatch as the coordination surface:
			// the system prompt renders "# Available Skills" for EVERY agent and tells
			// it to load a matching skill (and to discover hidden ones with
			// skill_search) before acting, while no profile allowlist names those
			// tools — so an allowlist-only worker is instructed to call a tool it
			// cannot see and silently skips skills instead. Loading a skill is
			// read-only, hence safe for every profile including the read-only ones.
			return true
		}
		if isExemptTool(name, exemptServer) {
			return true // repository-reading infrastructure (see allowlistExemptServer)
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

// ToolVisibilityFunc returns a per-agent resolver reporting a tool's effective
// visibility tier ("full" | "summary" | "name-only" | "hidden"), reflecting the
// workspace ToolVisibility overrides applied in buildRegistry. The claude-cli tier
// classifier (api.cliTier) consults it so the CLI core/extended split honors the
// same 4-tier model the native path uses: full → eager (core/alwaysLoad), summary/
// name-only → deferred (extended), hidden → not advertised. Built once per call.
func (r *Runtime) ToolVisibilityFunc(ctx context.Context, agent db.Agent) func(name string) string {
	reg := r.buildRegistry(ctx, agent)
	return reg.VisibilityOf
}

// ToolAllowedFunc returns a per-agent predicate reporting whether a tool is
// offered to this agent — workspace-active (not in the workspace DisabledTools)
// AND not on the agent denylist AND permitted by its allowlist — the SAME gate
// ToolCatalog applies on the native path. Unlike toolFilter it is ALWAYS non-nil:
// when nothing is restricted it reports every tool allowed. The claude-cli
// Interaction bridge consults it so a workspace-disabled or agent-blocked tool
// (e.g. PowerShell disabled to force Bash) is neither advertised in the bridge's
// tools/list nor placed in the CLI allowlist — closing the gap where the bridge
// exposed tools the native loop would have filtered out.
func (r *Runtime) ToolAllowedFunc(ctx context.Context, agent db.Agent) func(name string) bool {
	filter := r.toolFilter(ctx, agent)
	if filter == nil {
		return func(string) bool { return true }
	}
	return filter
}

// LazyToolsCatalogBlock renders the "Available Tools (load on demand)" system-
// prompt section for an agent: the name + summary of every lazy tool it may
// activate (self-management + MCP, minus its denylist). Returns "" when none.
// Part of the cached static prefix (stable per agent/workspace tool config).
func (r *Runtime) LazyToolsCatalogBlock(ctx context.Context, agent db.Agent) string {
	reg := r.buildRegistry(ctx, agent)
	filter := r.toolFilter(ctx, agent)
	// A CLI-provider agent (claude-cli, codex-cli) reaches these deferred (extended)
	// built-ins as MCP tools and loads them via TionHarness's gateway activate_tools
	// (Doc 52) — NOT the CLI's own tool search, which cannot find a tool that is not
	// advertised yet. Render the block in CLI form for it (namespaced names +
	// activate_tools). The provider KIND is passed through rather than a bool because
	// the CLI dialects differ in which built-ins they bridge (see catalogDisplayName);
	// the empty provider is the keyless claude-cli default.
	// Visible lazy tools are enumerated; the hidden self-management suite is folded
	// into a single skill pointer (rendered when hiddenCount > 0).
	return renderLazyToolCatalog(reg.VisibleLazyCatalog(filter), reg.HiddenLazyCount(filter), r.agentProviderKind(agent), reg.ServerDescriptions(), lazyBundleCounts(reg, filter))
}

// lazyBundleCounts returns bundle key -> member count over the LAZY catalog only,
// i.e. exactly the bundles activate_tools can open (buildRegistry hands the tool
// an index built with the SAME agent filter, and the tool drops members that are
// not lazy). Eager and agent-blocked tools are excluded so the advertised count
// matches the listing the agent actually gets.
func lazyBundleCounts(reg *tools.Registry, filter func(string) bool) map[string]int {
	lazy := map[string]bool{}
	for _, d := range reg.LazyCatalog(filter) {
		lazy[d.Name] = true
	}
	if len(lazy) == 0 {
		return nil
	}
	out := map[string]int{}
	for key, members := range reg.BundleIndex(filter) {
		n := 0
		for _, m := range members {
			if lazy[m] {
				n++
			}
		}
		if n > 0 {
			out[key] = n
		}
	}
	return out
}

// lazyCatalogMCPListLimit caps how many MCP (namespaced) lazy tools are listed
// individually in the load-on-demand catalog block. Above it, MCP tools are
// summarised per server with a count and the model is pointed at tool_search —
// this keeps the cached system-prompt prefix lean in MCP-heavy workspaces, where
// a single server can expose hundreds of tools. Built-in lazy tools (the
// self-management family) are always listed in full: they are few and high-value.
// This is the TionHarness analogue of the Anthropic "tool search" pattern — search
// instead of enumerate once the catalog grows large.
const lazyCatalogMCPListLimit = 50

// cliLazyBridgeExcluded names built-in lazy tools NOT advertised to a CLI-provider
// Interaction MCP bridge, so the CLI-form catalog must not list them. Mirrors
// tools.bridgeExcluded — keep in sync. run_subagent is eager (never in the lazy
// catalog), so WebFetch is the only one that normally surfaces here.
var cliLazyBridgeExcluded = map[string]bool{
	"WebFetch":     true, // claude-cli has its own native WebFetch (see claudeOnlyBridgeExclusions)
	"WebSearch":    true, // claude-cli has its own native WebSearch (defensive: eager, so not normally lazy)
	"run_subagent": true, // bridged explicitly via interactionToolSpecs, not the lazy path
	"run_code":     true, // code-execution mode is native-path-only (mirrors tools.bridgeExcluded)
	// deactivate_tools is a TionHarness-native meta-tool (paired with activate_tools);
	// the CLI uses its OWN ToolSearch, so this is never bridged — keep it out of the
	// CLI catalog even though it is name-only on the native path.
	"deactivate_tools": true,
}

// claudeOnlyBridgeExclusions narrows cliLazyBridgeExcluded to the entries that are
// withheld ONLY because claude-cli already ships an equivalent native. codex-cli
// ships no native WebFetch and keeps its own web_search OFF by default, so for it
// these are bridged (Runtime.BridgeTools advertises them) and must appear in the
// catalog — otherwise the agent has no web tool at all and starts inventing sources.
// Every other entry (run_subagent, run_code, deactivate_tools) stays excluded for
// BOTH dialects: those are about dispatch context, not about a CLI-native twin.
var claudeOnlyBridgeExclusions = map[string]bool{
	"WebFetch":  true,
	"WebSearch": true,
}

// cliBridgeExcludes reports whether a built-in is withheld from the CLI bridge for
// the given provider KIND. provider "" is the keyless claude-cli default, so an
// unknown/empty provider keeps the historical (claude-cli) exclusions.
func cliBridgeExcludes(name, provider string) bool {
	if !cliLazyBridgeExcluded[name] {
		return false
	}
	if provider == providerKindCodexCLI && claudeOnlyBridgeExclusions[name] {
		return false
	}
	return true
}

// catalogDisplayName maps a registry tool name to the identifier the target agent
// must actually call. Native agents call the bare/registry name as-is. A CLI agent
// reaches everything as MCP tools, so the name is namespaced: a namespaced MCP tool
// (server__tool) gains the CLI's "mcp__" prefix, and a built-in gains the
// Interaction MCP prefix. Returns ok=false for built-ins that CLI dialect does not
// bridge, so the caller skips them — which entries those are depends on the
// provider (see cliBridgeExcludes). provider is a provider KIND; "" is the keyless
// claude-cli default and native-API kinds take the bare name.
func catalogDisplayName(name, provider string) (string, bool) {
	if !isCLIProviderKind(provider) {
		return name, true
	}
	if _, _, ok := mcp.SplitNamespaced(name); ok {
		return mcp.NamespaceTool("mcp", name), true // server__tool → mcp__server__tool
	}
	if cliBridgeExcludes(name, provider) {
		return "", false // not bridged to this CLI (it has its own equivalent)
	}
	// Lazy built-ins are the EXTENDED tier (deferred via the CLI's ToolSearch), so
	// they are namespaced under the extended server key. Eager built-ins never reach
	// this catalog (they are inlined on the alwaysLoad core server).
	return extendedToolPrefix + name, true // built-in via the Interaction MCP bridge (extended tier)
}

// renderLazyToolCatalog builds the load-on-demand tool catalog block from the
// VISIBLE lazy tool defs (name + description). Built-in lazy tools are always
// listed; namespaced MCP tools are listed individually only while under
// lazyCatalogMCPListLimit, otherwise summarised per server. hiddenCount > 0 appends
// a single pointer to the `tionharness-self-management` skill in place of enumerating
// the hidden suite. Returns "" when there is nothing to show.
//
// provider is the agent's provider KIND. A CLI kind renders the block in CLI form:
// names are namespaced (mcp__tionharness_interaction__<name> for built-ins,
// mcp__<server>__<tool> for MCP) and loaded via the CLI's own ToolSearch — NOT
// TionHarness's native activate_tools (the CLI has neither activate_tools nor
// tool_search). Built-ins that dialect does not bridge (claude-cli's own WebFetch/
// WebSearch) are dropped. This mirrors how the skills block
// (CatalogBlockForAgentTool) already adapts to the CLI.
// bundles (key -> member count) adds ONE discovery line naming the openable
// bundles; empty/nil emits nothing, so a workspace without bundles renders the
// byte-identical block it did before. The line is native-only: the CLI path's
// activate_tools is the gateway one, whose bundle membership is derived from the
// run's candidate set rather than this registry, so advertising these exact
// counts there would be misleading.
func renderLazyToolCatalog(lazy []providers.ToolDef, hiddenCount int, provider string, serverDesc map[string]string, bundles map[string]int) string {
	if len(lazy) == 0 && hiddenCount == 0 {
		return ""
	}
	cli := isCLIProviderKind(provider)
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
	switch {
	case cli:
		// Gateway dynamic surface (Doc 52): the built-in extended tools below are NOT in
		// your tool list yet — you load them by calling `activate_tools` (a core tool),
		// which registers them and makes them callable the SAME turn. This is the
		// TionHarness path, NOT the CLI's own ToolSearch (which cannot find an un-advertised
		// tool). External MCP server tools (mcp__<server>__…) still load via ToolSearch.
		b.WriteString("These TionHarness tools are DEFERRED (not yet in your tool list). To use one, call " +
			"`activate_tools` with its name(s) (the namespaced name shown below, or its bare form) — it becomes " +
			"callable immediately. Activate everything you expect to need in one call. (External MCP server tools " +
			"named `mcp__<server>__…` load with the `ToolSearch` tool instead.)\n")
	default:
		b.WriteString("These tools are NOT loaded yet: an entry with no summary is deferred — use `tool_search` " +
			"to find what a name does. To call any of them, first `activate_tools` with its exact name(s); the " +
			"schema arrives on your next step. Activate everything you expect to need in one call.\n")
	}
	for _, d := range builtin {
		if name, ok := catalogDisplayName(d.Name, provider); ok {
			writeLazyToolLine(&b, name, d.Description)
		}
	}

	switch {
	case len(mcpTools) == 0:
		// nothing more to add
	case len(mcpTools) <= lazyCatalogMCPListLimit:
		// The entries below sit under the activate_tools sentence but do NOT load
		// that way on the CLI path; without this separator agents call
		// activate_tools on an mcp__… name, get rejected, and retry.
		if cli {
			b.WriteString("\nThe entries below are EXTERNAL MCP tools: load them with `ToolSearch` " +
				"(`select:<name>,<name>`), not `activate_tools`.\n")
		}
		for _, d := range mcpTools {
			if name, ok := catalogDisplayName(d.Name, provider); ok {
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
		// findHint carries its own backticks so the CLI form can spell out the
		// ToolSearch query syntax (`select:<name>`); without it agents call
		// ToolSearch with an invented parameter and get an InputValidationError.
		findHint, actHint := "`tool_search(\"keyword\")`", "activate_tools"
		if cli {
			findHint, actHint = "`ToolSearch` (`select:<name>,<name>`)", "load"
		}
		fmt.Fprintf(&b, "\n%d more tools are available from MCP servers but not listed individually "+
			"(to save context). Find one with %s, then `%s` it. Servers:\n", len(mcpTools), findHint, actHint)
		for _, srv := range order {
			label := srv
			if cli {
				label = "mcp__" + srv
			}
			if desc := strings.TrimSpace(serverDesc[srv]); desc != "" {
				fmt.Fprintf(&b, "- `%s` — %d tools — %s\n", label, counts[srv], desc)
			} else {
				fmt.Fprintf(&b, "- `%s` — %d tools\n", label, counts[srv])
			}
		}
	}

	if !cli && len(bundles) > 0 {
		keys := make([]string, 0, len(bundles))
		for k := range bundles {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s (%d)", k, bundles[k]))
		}
		fmt.Fprintf(&b, "\nBundles: %s — open one with `activate_tools(\"<bundle key>\")` to see its members' "+
			"summaries without loading any schema.\n", strings.Join(parts, ", "))
	}

	// Self-management suite: kept out of the per-turn enumeration to save context.
	// Point the model at the skill (which documents the full catalog + how to load
	// them) and at the search tool as the quick path. The skill + search names are
	// namespaced for the CLI, bare for native.
	if hiddenCount > 0 {
		skillTool, findHint, actHint := "use_skill", "`tool_search(\"keyword\")`", "activate_tools"
		if cli {
			skillTool = interactionToolPrefix + "use_skill"
			findHint, actHint = "`ToolSearch` (`select:<name>,<name>`)", "load"
		}
		// "memory" is deliberately absent from this list: the memory subsystem was
		// removed on 2026-07-05 and naming it here advertised tools that do not exist.
		fmt.Fprintf(&b, "\n%d self-management tools (agents, flows, schedules, tasks, hooks, MCP servers, "+
			"skills, workspaces, app settings, your own prompts/config, secrets, logs) are not listed here "+
			"to save context. Load the `tionharness-self-management` skill (via `%s`) for the full catalog, "+
			"or find one with %s — then `%s` the names you need.\n", hiddenCount, skillTool, findHint, actHint)
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
// effective visibility tier after all marks are applied — code defaults
// (self-management, MCP, etc.) AND the workspace per-tool overrides. The returned
// map is tool name → one of tools.Visibility* ("full" | "summary" | "name-only" |
// "hidden"); it drives the tools screen's tier selector and chips.
func (r *Runtime) WorkspaceToolCatalogWithState(ctx context.Context) ([]providers.ToolDef, map[string]string) {
	reg := r.buildRegistry(ctx, db.Agent{})
	defs := reg.Defs(nil)
	vis := make(map[string]string, len(defs))
	for _, d := range defs {
		vis[d.Name] = reg.VisibilityOf(d.Name)
	}
	return defs, vis
}

// ActiveToolCatalogWithState is ActiveToolCatalog plus each tool's
// WORKSPACE-EFFECTIVE visibility tier — code defaults + the workspace override
// map, with NO agent overrides applied (it builds against a zero agent).
//
// That is exactly the "default" an agent's override is measured against: the
// agent tools screen shows only the tools whose per-agent tier differs from this
// baseline, so the user sees the diff rather than 140 unchanged rows.
func (r *Runtime) ActiveToolCatalogWithState(ctx context.Context) ([]providers.ToolDef, map[string]string) {
	reg := r.buildRegistry(ctx, db.Agent{})
	filter := func(name string) bool { return true }
	if disabled := r.workspaceDisabledSet(ctx); disabled != nil {
		filter = func(name string) bool { return !disabled[name] }
	}
	defs := reg.Defs(filter)
	vis := make(map[string]string, len(defs))
	for _, d := range defs {
		vis[d.Name] = reg.VisibilityOf(d.Name)
	}
	return defs, vis
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

// capStepInput bounds a code-mode call's args for the nested trace card. The
// UI trace is not model context, but a pathological multi-KB argument blob
// would still bloat the persisted step JSON — keep a readable prefix.
func capStepInput(args json.RawMessage) json.RawMessage {
	const maxLen = 2048
	if len(args) <= maxLen {
		return args
	}
	// Truncated JSON would fail to render as structured input; fall back to a
	// quoted string payload carrying the readable prefix.
	quoted, err := json.Marshal(string(args[:maxLen]) + "…(truncated)")
	if err != nil {
		return nil
	}
	return quoted
}

// humanStepBytes renders a byte count for the nested trace row's output line.
func humanStepBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	default:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
}

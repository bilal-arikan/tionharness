// Package agent implements TionSwarm's multi-agent ("swarm") runtime: it owns each
// agent's provider calls, tool loop, delegation, cron scheduler and the headless
// autonomous entry points (schedule/spawn/flow).
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/market"
	"github.com/bilal-arikan/tionswarm/internal/mcp"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/secrets"
	"github.com/bilal-arikan/tionswarm/internal/skills"
	"github.com/bilal-arikan/tionswarm/internal/tools"
)

// Runtime owns the lifecycle of all autonomous agent workers.
type Runtime struct {
	db        *db.DB
	providers *providers.Registry
	tun       *Tunables
	logger    *slog.Logger

	// workDir is this workspace's sandbox root for built-in filesystem/shell
	// tools. Every fs/shell tool call is confined to it.
	workDir string

	// vault is this workspace's secret store, exposed to agents through the
	// secret_list / secret_get built-in tools. May be nil (no secret tools).
	vault *secrets.Vault

	// bus + workspace identity let autonomous events (task/schedule)
	// be published with enough context for the UI to deep-link on click.
	bus    *events.Bus
	wsID   string
	wsName string

	// logs is the process-wide ring buffer of captured log entries, exposed to
	// agents through the read_logs self-management tool. May be nil.
	logs *logbuf.Buffer

	// skills resolves reusable skill instruction sets (global/workspace/project
	// tiers) and backs the use_skill tool + the Available Skills prompt block.
	skills *skills.Store

	// market is the in-app marketplace: a file-based registry of shareable packs
	// (skill/agent/provider/flow) that can be browsed, installed and published.
	market *market.Store

	// reloadSched re-reads schedules into the cron scheduler after an agent
	// creates/edits/deletes one via a self-management tool. Wired by the
	// workspace manager once the scheduler exists; nil before then (no-op).
	reloadSched func(context.Context) error

	// runSched fires a schedule immediately (the scheduler's RunNow), backing the
	// run_schedule self-management tool. Wired by the workspace manager once the
	// scheduler exists; nil before then.
	runSched func(context.Context, string) error

	// turnHooks are called (detached, non-blocking) whenever an agent turn finishes
	// on any path (chat/spawn/schedule/wake). The workspace manager wires the
	// AutomationEngine (tag-triggered automations) and the CoordinationEngine
	// (worker → coordinator <task-notification> feedback) here. Empty before wiring;
	// guarded by turnHooksMu since AddTurnHook runs during boot while
	// FireTurnFinished may already be firing.
	turnHooks   []func(context.Context, TurnFinished)
	turnHooksMu sync.RWMutex
	// failedTurnHooks fire on turns that ended with an ERROR (see FireTurnFailed):
	// only the automation engine subscribes, so repair automations can react to
	// the failing turn itself. Guarded by the same mutex.
	failedTurnHooks []func(context.Context, TurnFinished)

	// coordSlots serializes turns per session for the coordinator/worker loop: one
	// slot per coordinator session so concurrent worker notifications never run two
	// coordinator turns at once, and notifications that arrive mid-turn coalesce
	// into the next single turn (they are already persisted as history). Keyed by
	// session id; value is *coordSlot. See NotifyCoordinator + coordination.go.
	coordSlots sync.Map

	// readTrackers holds one *tools.ReadTracker per session id, the freshness
	// baseline the Edit/Write guard compares against. Session-scoped and persistent
	// across turns (a file read in an earlier turn stays a valid baseline for a later
	// edit). Keyed by session id; value is *tools.ReadTracker. Gated by
	// Tunables.FileFreshnessGuard; unstamped-session builds (catalog/preview) get nil.
	readTrackers sync.Map

	// cbmIndexed guards the best-effort codebase-memory auto-index so a repo is
	// indexed into this workspace's isolated store at most once per (cwd, store) per
	// process. Keyed by "<cwd>|<store>"; value is bool. See EnsureCodebaseIndexed.
	cbmIndexed sync.Map

	// shellMgrs holds one *tools.ShellManager per session id, tracking that session's
	// background shells (run_in_background) so their output can be polled and they can
	// be stopped across turns. Session-scoped and persistent across turns; keyed by
	// session id, value is *tools.ShellManager. Unstamped-session builds (catalog/
	// preview) get nil, which disables background execution for that build.
	shellMgrs sync.Map

	// workerCancels tracks in-flight worker turns so stop_worker can cancel one.
	// Keyed by worker session id; value is *workerCtl (cancel func + stopped flag,
	// so a cancelled turn reports "killed" rather than "failed"). Populated by
	// runWorker for its lifetime.
	workerCancels sync.Map

	// coordRunFn, when non-nil, replaces runCoordinatorTurn in the per-session queue
	// drain — a test seam so the queue's serialization/coalescing can be exercised
	// without a live provider. Nil in production (the real turn runs).
	coordRunFn func(coordSessionID string)

	// profileWorkerMu serializes find-or-create of the persisted profile-worker
	// agents (worker:explore/coder/reviewer) so two concurrent spawn_worker calls
	// with the same profile target never create duplicate agents.
	profileWorkerMu sync.Mutex

	// settingsBridge backs the get_settings / update_settings self-management
	// tools: read and live-apply the application-wide settings. Wired by the
	// workspace manager once the api server exists; nil before then (tools off).
	settingsBridge tools.SettingsBridge

	// workspaceBridge backs the workspace self-management tools (list/create/
	// rename/delete_workspace): cross-workspace operations through the manager.
	// Wired by the workspace manager once the api server exists; nil before then
	// (tools off).
	workspaceBridge tools.WorkspaceBridge

	// autoInteract wires an Interaction MCP endpoint for headless (non-chat) CLI
	// turns so scheduler/spawn agents reach use_skill/shell/self-manage.
	// Set by the workspace manager once the api server exists; nil before then.
	autoInteract AutonomousInteraction

	// wakeTurn runs a history-aware chat turn for a self-wake (schedule_wake), so
	// the woken agent continues with the full conversation instead of just the
	// wake prompt. Set by the workspace manager once the api server exists; nil
	// before then (deliverWake falls back to the prompt-only invoke).
	wakeTurn WakeTurnFunc

	// paused is this workspace's autonomy brake (set from per-workspace
	// settings); when true, autonomous calls are rejected like the global one.
	paused atomic.Bool

	// instructions is this workspace's free-form guidance (set from per-workspace
	// settings), appended to every agent's static system prompt.
	instructions atomic.Pointer[string]

	// defaultWorkDir is this workspace's user-chosen default working directory (cwd)
	// for new sessions, set from per-workspace settings. Empty = fall back to
	// workDir (the physical workspace dir). A session's own WorkingDir overrides it.
	defaultWorkDir atomic.Pointer[string]

	// Cross-session awareness config, set from per-workspace settings: whether the
	// feature is on (gates both the pushed context block and the list_sessions
	// pull tool), whether to inject every turn (vs only a session's first turn),
	// and how many past sessions to list.
	sessionCtxEnabled   atomic.Bool
	sessionCtxEveryTurn atomic.Bool
	sessionCtxRecent    atomic.Int64

	// codebaseMemoryEnabled gates the whole codebase-memory capability system for
	// this workspace: the prompt hint block, the per-workspace isolated store env,
	// the cwd auto-index, and the codebase_workspace_search tool. Default on;
	// workspace settings can switch the entire feature off. See capabilities.go.
	codebaseMemoryEnabled atomic.Bool

	// activeSessions tracks sessions currently executing an autonomous invoke
	// (schedule / spawn). Keyed by session id; value is struct{}.
	// Used by the executions feed to show a live "running" indicator for
	// autonomous runs that aren't chat-streaming turns.
	activeSessions sync.Map

	// spawnActive counts the spawned sessions currently running their background
	// turn — the fire-and-forget concurrency guard (capped by SpawnMaxConcurrent).
	spawnActive atomic.Int64

	// mcpPool holds this workspace's persistent MCP connections (one live session
	// per enabled server). It replaces dial-per-operation: the per-turn catalog
	// builds reuse live sessions (no gateway session churn) and a server's
	// session state — e.g. the gateway's activate_tools — survives across calls.
	// Closed via CloseMCP when the workspace is torn down.
	mcpPool *mcp.Pool

	// cliSessions holds long-lived claude-cli processes (one per session+agent) when
	// the ClaudePersistentSession setting is on, so warm turns ship only the new user
	// message. Off by default; nil-safe (recordedComplete falls back to one-shot
	// Complete). Closed via CloseMCP on workspace teardown.
	cliSessions *providers.CLISessionPool

	// cacheProbes holds the per-session last-known prompt-cache state (prefix hash +
	// model + warmth) used to detect and attribute a prompt-cache break turn-to-turn
	// (P4, cachebreak.go). Keyed by session id; zero value ready. Best-effort
	// telemetry only — never gates a turn.
	cacheProbes sync.Map
}

// trackSession marks a session as actively running an autonomous invoke.
func (r *Runtime) trackSession(id string) { r.activeSessions.Store(id, struct{}{}) }

// untrackSession removes the running marker when an invoke finishes.
func (r *Runtime) untrackSession(id string) { r.activeSessions.Delete(id) }

// ActiveSessionIDs returns the session ids currently running autonomous invokes.
func (r *Runtime) ActiveSessionIDs() []string {
	var ids []string
	r.activeSessions.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids
}

// SetPaused toggles this workspace's autonomy brake.
func (r *Runtime) SetPaused(p bool) { r.paused.Store(p) }

// Paused reports this workspace's autonomy brake state.
func (r *Runtime) Paused() bool { return r.paused.Load() }

// SetInstructions updates this workspace's agent-wide guidance.
func (r *Runtime) SetInstructions(s string) { r.instructions.Store(&s) }

// SetDefaultWorkDir updates this workspace's default working directory for new
// sessions (set from per-workspace settings). Empty clears it (back to workDir).
func (r *Runtime) SetDefaultWorkDir(s string) { r.defaultWorkDir.Store(&s) }

// WorkspaceDefaultDir returns the working dir used when a session has no override:
// the configured default working dir when set and valid, else the physical
// workspace dir. This is the fallback for effectiveWorkDir and the cwd badge.
func (r *Runtime) WorkspaceDefaultDir() string {
	if p := r.defaultWorkDir.Load(); p != nil {
		if d := strings.TrimSpace(*p); d != "" {
			if info, err := os.Stat(d); err == nil && info.IsDir() {
				return d
			}
		}
	}
	return r.workDir
}

// SetSessionContext updates this workspace's cross-session awareness config.
func (r *Runtime) SetSessionContext(enabled, everyTurn bool, recent int) {
	r.sessionCtxEnabled.Store(enabled)
	r.sessionCtxEveryTurn.Store(everyTurn)
	r.sessionCtxRecent.Store(int64(recent))
}

// SessionContextEnabled reports whether cross-session awareness is on for this
// workspace (gates the pushed block and the list_sessions tool).
func (r *Runtime) SessionContextEnabled() bool { return r.sessionCtxEnabled.Load() }

// SessionContextEveryTurn reports whether the block is injected every turn.
func (r *Runtime) SessionContextEveryTurn() bool { return r.sessionCtxEveryTurn.Load() }

// SessionContextRecentCount returns how many past sessions to list (default when unset).
func (r *Runtime) SessionContextRecentCount() int {
	if n := int(r.sessionCtxRecent.Load()); n > 0 {
		return n
	}
	return DefaultSessionContextRecent
}

// SetCodebaseMemory toggles the codebase-memory capability system for this
// workspace (hint block + isolated store + auto-index + workspace-search tool).
func (r *Runtime) SetCodebaseMemory(enabled bool) { r.codebaseMemoryEnabled.Store(enabled) }

// CodebaseMemoryEnabled reports whether the codebase-memory capability system is on
// for this workspace.
func (r *Runtime) CodebaseMemoryEnabled() bool { return r.codebaseMemoryEnabled.Load() }

// NewRuntime constructs the runtime. tun carries the process-wide tunables
// (autonomy pause, title-model override) shared across all workspace runtimes.
// workDir is the workspace sandbox root for built-in filesystem/shell tools.
// bus + wsID/wsName let autonomous events be published with workspace context
// (bus may be nil, in which case publishing is a no-op). vault is this
// workspace's secret store handed to the secret_* tools (may be nil).
func NewRuntime(database *db.DB, registry *providers.Registry, tun *Tunables, workDir string, vault *secrets.Vault, bus *events.Bus, wsID, wsName string, logs *logbuf.Buffer, logger *slog.Logger) *Runtime {
	// Seed the shipped default skills into the global dir (idempotent, never
	// overwrites) so every workspace inherits the TionSwarm guide skills.
	_ = skills.EnsureDefaults(globalSkillsDir())
	// The marketplace has no bundled/workspace tiers: packs live only in the
	// global market dir (<DataDir>/market) and remote registries. No seeding.
	r := &Runtime{
		db:          database,
		providers:   registry,
		tun:         tun,
		workDir:     workDir,
		vault:       vault,
		bus:         bus,
		wsID:        wsID,
		wsName:      wsName,
		logs:        logs,
		logger:      logger,
		skills:      skills.New(globalSkillsDir(), workspaceSkillsDir(workDir)),
		market:      market.New(marketGlobalDir(), workspaceLedgerDir(workDir)),
		mcpPool:     mcp.NewPool(),
		cliSessions: providers.NewCLISessionPool(),
	}
	// The codebase-memory capability defaults ON; workspace settings (loadSettings)
	// override it at boot. Seeded here so bare runtimes (before settings apply) still
	// behave as "on" rather than silently off.
	r.codebaseMemoryEnabled.Store(true)
	// Surface MCP connection lifecycle (dial / re-dial / list_changed) in the
	// in-app Logs screen; the persistent pool is otherwise opaque.
	r.mcpPool.SetLogger(logger)
	// Same rationale for the persistent claude-cli pool: surface cold-start reason
	// (config-change / dead-process), warm reuse, and process death in Logs so a
	// surprise cold turn is diagnosable instead of silent.
	r.cliSessions.SetLogger(logger)
	return r
}

// MCPPool exposes this workspace's persistent, ref-counted MCP connection pool so the
// external gateway (Doc 52 Faz 3) can SHARE it — an internal agent turn and an external
// gateway client then reuse one backend connection per server instead of spawning
// duplicate subprocesses (#11). The pool is workspace-owned; callers must NOT Close it.
func (r *Runtime) MCPPool() *mcp.Pool { return r.mcpPool }

// CloseMCP terminates this workspace's persistent MCP connections. Called when
// the workspace is deleted or the manager shuts down.
func (r *Runtime) CloseMCP() {
	if r.mcpPool != nil {
		r.mcpPool.Close()
	}
	if r.cliSessions != nil {
		r.cliSessions.Close()
	}
}

// HasWarmCLISession reports whether the session has a warm (persistent-pool)
// claude-cli process kept alive between turns. Nil-safe (pool may be unset).
func (r *Runtime) HasWarmCLISession(sessionID string) bool {
	return r.cliSessions != nil && r.cliSessions.AliveForSession(sessionID)
}

// DropWarmCLISession recycles the session's warm claude-cli process(es) so the next
// turn cold-restarts fresh. Returns how many were dropped. Nil-safe.
func (r *Runtime) DropWarmCLISession(sessionID string) int {
	if r.cliSessions == nil {
		return 0
	}
	return r.cliSessions.DropSession(sessionID)
}

// globalSkillsDir is TionSwarm's data-dir-level global skills directory
// (<DataDir>/skills, default ~/.tionswarm/skills). Deliberately under TionSwarm's
// OWN data dir — not the cross-tool ~/.agents/skills convention — so TionSwarm's
// global skills stay isolated from other agent tools that share that directory.
// Honors TIONSWARM_DATA_DIR so a custom data dir is respected (mirrors config).
func globalSkillsDir() string {
	if d := os.Getenv("TIONSWARM_DATA_DIR"); d != "" {
		return filepath.Join(d, "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tionswarm", "skills")
}

// workspaceSkillsDir is this workspace's skills directory (<workspace>/skills),
// a sibling of store/, config/ and workspace/. Empty when workDir is unknown.
// This is the workspace skill tier for TionSwarm's use_skill bridge. It is
// deliberately NOT the claude CLI's native skills dir (<CLAUDE_CONFIG_DIR>/skills):
// the native Skill tool stays disabled (see climcp.go) so use_skill is the single
// skill path serving both this tier and the global tier.
func workspaceSkillsDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "skills")
}

// claudeHomeDir is this workspace's per-workspace claude-cli config home
// (<workspace>/claude-home), exported into the CLI subprocess as CLAUDE_CONFIG_DIR
// so each workspace drives the CLI against its own skills/settings/login. Empty
// when workDir is unknown (the provider then keeps its global-default config dir).
func (r *Runtime) claudeHomeDir() string { return workspaceClaudeHomeDir(r.workDir) }

// Skills returns this runtime's skill store (never nil after construction).
func (r *Runtime) Skills() *skills.Store { return r.skills }

// Market returns this runtime's marketplace store (never nil after construction).
func (r *Runtime) Market() *market.Store { return r.market }

// WorkspaceSkillsDir is the workspace's skills directory, where the market
// installs skill packs. Exposed so the API layer can pass it to InstallSkill.
func (r *Runtime) WorkspaceSkillsDir() string { return workspaceSkillsDir(r.workDir) }

// WorkspaceHookScriptsDir is where imported hook packs materialise their bundled
// scripts (one subdir per hook slug). ${CLAUDE_PLUGIN_ROOT} in an imported hook
// command is rewritten to that subdir. Sibling of the skills dir so it is
// per-workspace and survives restarts.
func (r *Runtime) WorkspaceHookScriptsDir() string {
	if r.workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(r.workDir), "hook-scripts")
}

// ShellEnabled reports whether the built-in shell tool may be offered. Exposed so
// the CLI Interaction MCP bridge gates the bridged shell exactly like the native
// path's shell gate.
func (r *Runtime) ShellEnabled() bool { return r.tun.ShellEnabled() }

// WorkDir returns this workspace's default working directory (the fs/shell base
// dir). Exposed so the API layer can show the effective cwd when a session has no
// per-session WorkingDir override.
func (r *Runtime) WorkDir() string { return r.workDir }

// AutonomousInteraction prepares an Interaction MCP endpoint for a headless
// (non-chat) CLI turn — scheduler / spawn / flow — so those agents
// reach the same use_skill / shell / self-management bridge that chat agents do.
// It returns an augmented context (carrying the endpoint) and a cleanup func that
// MUST be called when the turn ends. Installed by the api server, which owns the
// run registry and the loopback endpoint. nil = no wiring (CLI agent falls back
// to its native tools, as before).
type AutonomousInteraction func(ctx context.Context, agent db.Agent, sessionID string) (context.Context, func())

// SetAutonomousInteraction wires the headless Interaction MCP setup. The
// workspace manager calls this for every runtime (existing + later-opened).
func (r *Runtime) SetAutonomousInteraction(fn AutonomousInteraction) { r.autoInteract = fn }

// WakeTurnFunc runs a full, history-aware chat turn for a self-wake: given the
// originating session (whose history already includes the wake prompt as the
// last user message) it composes the same rich request an interactive chat turn
// gets — conversation history, author labels, memory, goal, summary — and runs
// the agentic loop, returning the reply text and its activity trace. Installed by
// the api server (which owns chat-turn composition); nil falls back to the
// prompt-only invoke.
type WakeTurnFunc func(ctx context.Context, agent db.Agent, sessionID, prompt string) (string, []TurnStep, error)

// SetWakeTurnRunner wires the history-aware wake-turn runner. The workspace
// manager calls this for every runtime (existing + later-opened).
func (r *Runtime) SetWakeTurnRunner(fn WakeTurnFunc) { r.wakeTurn = fn }

// WorkspaceID returns this runtime's workspace id, so a wake-turn runner bound to
// the runtime can resolve its workspace (DB, settings) from the manager.
func (r *Runtime) WorkspaceID() string { return r.wsID }

// NewShellRunner returns a closure that runs a shell command through the
// workspace-sandboxed shell tool (PowerShell on Windows, /bin/sh elsewhere), for
// the claude-cli Interaction MCP bridge — so a CLI agent runs commands through
// TionSwarm's own shell (sandboxed, bounded, permission/hook-gated) instead of the
// CLI's native POSIX Bash. Returns nil when shell is disabled or no sandbox is
// configured, so the bridge advertises shell only when it can honour it.
func (r *Runtime) NewShellRunner() func(ctx context.Context, args json.RawMessage) (string, error) {
	if !r.tun.ShellEnabled() {
		return nil
	}
	if !tools.NewSandbox(r.workDir).Ready() {
		return nil
	}
	// Resolve the working dir per call so the bridged shell honours the session's
	// WorkingDir override (the ctx carries the session id), matching the native
	// path. Unconfined, like an interactive turn. The CLI path exposes ONE shell:
	// the OS-native one (PowerShell on Windows, Bash on Unix) — the same identity the
	// bridge advertises (see interactionToolSpecs) and dispatches (callShell).
	return func(ctx context.Context, args json.RawMessage) (string, error) {
		sb := tools.NewSandbox(r.effectiveWorkDir(ctx))
		if runtime.GOOS == "windows" {
			return tools.NewPowerShellTool(sb).Call(ctx, args)
		}
		return tools.NewShellTool(sb).Call(ctx, args)
	}
}

// MarketGlobalDir exposes the data-dir-level global market directory so the api
// layer can build a workspace-independent market store (used by the workspace
// template picker, which must work even with zero workspaces during onboarding).
func MarketGlobalDir() string { return marketGlobalDir() }

// marketGlobalDir is TionSwarm's data-dir-level global market directory
// (<DataDir>/market, default ~/.tionswarm/market). Mirrors globalSkillsDir.
func marketGlobalDir() string {
	if d := os.Getenv("TIONSWARM_DATA_DIR"); d != "" {
		return filepath.Join(d, "market")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tionswarm", "market")
}

// workspaceLedgerDir is the workspace root where the market install ledger
// (installed.json) is kept — per-workspace, since installs (skills/agents/…) are
// per-workspace. The market itself no longer keeps a per-workspace pack tier;
// only this small tracking file lives here. Empty when workDir is unknown.
func workspaceLedgerDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Dir(workDir)
}

// SkillsCatalogBlockForAgent renders the Available Skills system-prompt section
// an agent sees: its assigned skills (in the agent's chosen order) plus every
// shared (on-demand) skill. Returns "" when neither exists. Skills are a shared
// library; agents pick from it — they never own skills.
func (r *Runtime) SkillsCatalogBlockForAgent(agent db.Agent) string {
	if r.skills == nil {
		return ""
	}
	// Name the use_skill tool exactly as THIS agent will see it. A claude-cli agent
	// reaches TionSwarm's built-ins through the Interaction MCP bridge, where they are
	// namespaced (mcp__tionswarm_interaction__use_skill). Advertising the bare name to
	// it makes the model emit an unqualified `use_skill` call the CLI rejects with
	// "No such tool available: use_skill" on the first turn (it recovers on retry by
	// finding the namespaced tool, but the wasted round-trip + error is avoidable).
	return r.skills.CatalogBlockForAgentTool(agent.Skills, skillToolNameFor(agent.Provider))
}

// skillToolNameFor returns the identifier the use_skill tool carries for an agent
// on the given provider: native (API) providers register the bare name, while
// claude-cli reaches it namespaced through the Interaction MCP bridge. The empty
// provider is the keyless claude-cli default. Custom providers are only ever
// OpenAI/Anthropic-compatible (native), so they take the bare name.
func skillToolNameFor(provider string) string {
	if provider == "" || provider == "claude-cli" {
		return interactionToolPrefix + skills.DefaultSkillTool
	}
	return skills.DefaultSkillTool
}

// LoadSkillForAgent returns a skill's full body for the CLI path (the Interaction
// MCP use_skill bridge), enforcing the SAME per-agent allowlist as the native
// use_skill built-in. It mirrors the agentSkillLib the native tool loop builds,
// so both provider paths advertise an identical contract over one skill store.
func (r *Runtime) LoadSkillForAgent(agent db.Agent, slug string) (string, error) {
	if r.skills == nil {
		return "", fmt.Errorf("skills are not available")
	}
	allow := r.skills.AllowedFor(agent.Skills)
	return agentSkillLib{store: r.skills, allow: allow}.Body(slug)
}

// SearchSkillsForAgent powers the CLI-path skill_search bridge: it searches the
// library but returns only skills the agent may load (assigned + shared), so a
// claude-cli agent can discover on-demand/conditional skills the same way native
// agents do via the skill_search tool. (SK-2)
func (r *Runtime) SearchSkillsForAgent(agent db.Agent, query string, limit int) []tools.SkillHit {
	if r.skills == nil {
		return nil
	}
	allow := r.skills.AllowedFor(agent.Skills)
	return agentSkillLib{store: r.skills, allow: allow}.SearchSkills(query, limit)
}

// SkillAllowedToolsForAgent returns the allowed-tools the named skill declares,
// for the CLI-path use_skill bridge to auto-grant — SK-3 parity with the native
// use_skill tool.
func (r *Runtime) SkillAllowedToolsForAgent(agent db.Agent, slug string) []string {
	if r.skills == nil {
		return nil
	}
	allow := r.skills.AllowedFor(agent.Skills)
	return agentSkillLib{store: r.skills, allow: allow}.AllowedTools(slug)
}

// BridgeTools builds the per-agent tool registry and returns the bridgeable
// (lazy built-in = self-management) tool schemas plus a dispatcher, for the CLI
// path's Interaction MCP bridge (CLI-3). claude-cli has no native activate_tools
// loop, so instead of lazy-loading these are advertised up front and dispatched
// straight through the same registry the native loop uses — identical behaviour
// and the same per-agent allowlist. Returns an empty catalog when self-management
// is off (no lazy built-ins are registered). The dispatcher runs any built-in by
// name (the catalog is the gate); unknown/foreign names return an error.
func (r *Runtime) BridgeTools(ctx context.Context, agent db.Agent) ([]providers.ToolDef, func(ctx context.Context, name string, args json.RawMessage) (string, error)) {
	reg := r.buildRegistry(ctx, agent)
	allow := r.toolFilter(ctx, agent)
	// Bridge ALL lazy built-ins INCLUDING the hidden tier. With the gateway dynamic
	// surface (Doc 52), the extended tier advertises nothing until the model activates a
	// tool, so bridging hidden schemas costs zero tokens up front — yet it makes hidden
	// tools discoverable (tool_search) and activatable in-turn, the CLI analogue of
	// native hidden tools. (The former cliBridgeSkipHidden POC that withheld them is
	// obsolete now that advertisement is on-demand.)
	defs := reg.BridgeableDefsFiltered(allow, false)
	// Bridge a few EAGER built-ins that the CLI would otherwise lack but that have
	// no native-loop context dependency (they only need r.mem / r.db, which reg
	// already holds). They stay eager on the native path — we just advertise them
	// to the CLI here; the call closure below dispatches them via reg.Call by name,
	// exactly like the lazy bridged tools. Gated to match buildRegistry's own gates.
	//   - conversation_search        : full-text history search (deeper than the
	//     list_sessions pull tool, which is already bridged).
	var extra []providers.ToolDef
	if r.SessionContextEnabled() {
		extra = append(extra, tools.NewConversationSearchTool(r.db).Def())
	}
	//   - read_session_debug : let a CLI agent read its OWN session's debug journal
	//     (timings, token spend, tool latency, anomalies) for self-improvement —
	//     the same always-on observability tool the native path gets. Only needs
	//     r.db; the session id is injected into the call ctx below.
	if r.tun.DebugJournalEnabled() {
		extra = append(extra, tools.NewReadSessionDebugTool(r.db).Def())
	}
	//   - get_session_info : read own-session metadata (title/tags/goal/role) — the
	//     session id is injected into the call ctx below, same as read_session_debug.
	extra = append(extra, tools.NewGetSessionInfoTool(r.db).Def())
	//   - update_user_preferences : persist durable user facts into the app-wide
	//     profile; only needs the settings bridge reg already dispatches through.
	if r.settingsBridge != nil {
		extra = append(extra, tools.NewUpdateUserPreferencesTool(r.settingsBridge).Def())
	}
	for _, d := range extra {
		if allow == nil || allow(d.Name) {
			defs = append(defs, d)
		}
	}
	// The bridged call closure runs on the Interaction server's request ctx, which
	// lacks the turn's current-session id; inject it (captured from the build ctx)
	// so session-scoped bridged tools (read_session_debug) default to this session.
	sid := SessionIDFrom(ctx)

	// Coordinator sessions (M2): bridge the spawn_worker/send_to_worker/stop_worker/
	// list_workers tools to the CLI path too, so a claude-cli coordinator can drive
	// workers. They dispatch outside reg (via the coordination runner), so they need
	// no registry registration. Only advertised when this session is a coordinator.
	var coordFuncs *tools.CoordinationFuncs
	if sid != "" && r.sessionRole(ctx) == "coordinator" {
		coordFuncs = r.coordinationFuncsFor(sid, agent.ID)
		for _, d := range coordinationBridgeDefs() {
			if allow == nil || allow(d.Name) {
				defs = append(defs, d)
			}
		}
	}

	call := func(ctx context.Context, name string, args json.RawMessage) (string, error) {
		if sid != "" {
			ctx = tools.WithCurrentSession(ctx, sid)
			ctx = WithSessionID(ctx, sid)
		}
		if coordFuncs != nil {
			if out, handled, err := dispatchCoordinationBridge(ctx, coordFuncs, name, args); handled {
				return out, err
			}
		}
		res := reg.Call(ctx, providers.ToolCall{Name: name, Input: args})
		if res.IsError {
			return "", fmt.Errorf("%s", res.Content)
		}
		return res.Content, nil
	}
	return defs, call
}

// agentSkillLib restricts the use_skill tool to an agent's selected slugs, so an
// agent cannot load a skill it has not been given.
type agentSkillLib struct {
	store *skills.Store
	allow map[string]bool
}

func (l agentSkillLib) Body(slug string) (string, error) {
	if !l.allow[slug] {
		return "", fmt.Errorf("skill %q is not enabled for this agent", slug)
	}
	return l.store.UseSkillBody(slug, l.allow)
}

// AllowedTools returns the tool-permission patterns the named skill declares,
// restricted to skills this agent may load. Powers SK-3 (loading a skill
// auto-grants its tools for the session).
func (l agentSkillLib) AllowedTools(slug string) []string {
	if !l.allow[slug] {
		return nil
	}
	sk, ok := l.store.Get(slug)
	if !ok {
		return nil
	}
	return sk.AlwaysAllow
}

// SearchSkills powers the skill_search tool: it searches the full library but
// returns only skills this agent may load (assigned + shared), so discovery never
// reveals a skill the agent could not then use. (SK-2)
func (l agentSkillLib) SearchSkills(query string, limit int) []tools.SkillHit {
	out := []tools.SkillHit{}
	for _, sk := range l.store.Search(query, 0) {
		if !l.allow[sk.Slug] {
			continue
		}
		out = append(out, tools.SkillHit{Slug: sk.Slug, Description: sk.Description, WhenToUse: sk.WhenToUse})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

// agentSkillWriter adapts *skills.Store to the tools.SkillWriter interface so the
// create_skill / delete_skill self-management tools can author workspace skills
// without the tools package importing the skills package. db (optional) lets
// DeleteSkill strip the removed slug from every agent's skill selection.
type agentSkillWriter struct {
	store *skills.Store
	db    *db.DB
}

// ValidateSkill adapts skills.Store.ValidateSkill onto the tools-layer view so the
// skill_validate tool stays decoupled from the skills package.
func (w agentSkillWriter) ValidateSkill(slug string) tools.SkillValidation {
	r := w.store.ValidateSkill(slug)
	return tools.SkillValidation{
		Found:    r.Found,
		Tier:     r.Tier,
		Path:     r.Path,
		Valid:    r.Valid,
		Errors:   r.Errors,
		Warnings: r.Warnings,
	}
}

func (w agentSkillWriter) CreateSkill(slug, name, description, whenToUse, group, body string, shared bool) error {
	_, err := w.store.Create(slug, skills.SkillInput{
		Name:        name,
		Description: description,
		WhenToUse:   whenToUse,
		Group:       group,
		Body:        body,
		Shared:      shared,
	})
	return err
}

// ImportSkill imports a Claude Code skill (local dir or github URL) into the
// workspace tier and maps the skills.ImportResult onto the tools view. (SK-IMP)
func (w agentSkillWriter) ImportSkill(source, location, slug string, shared bool) (tools.SkillImportResult, error) {
	_, res, err := w.store.ImportFromSource(source, location, slug, shared)
	if err != nil {
		return tools.SkillImportResult{}, err
	}
	return tools.SkillImportResult{Slug: res.Slug, Warnings: res.Warnings, Files: res.Files}, nil
}

// UpdateSkill edits a workspace skill in place. Each pointer field is applied
// only when non-nil (partial update), merging over the skill's current values
// so the agent can change just the body. Restricted to workspace-tier skills so
// bundled/global skills can't be overwritten (parity with DeleteSkill).
func (w agentSkillWriter) UpdateSkill(slug string, name, description, whenToUse, group, body *string, shared *bool) error {
	cur, ok := w.store.Get(slug)
	if !ok {
		return fmt.Errorf("no skill with slug %q (check the skill catalog)", slug)
	}
	if cur.Source != skills.SourceWorkspace {
		return fmt.Errorf("skill %q is a %s skill and cannot be edited (only workspace skills are editable)", slug, cur.Source)
	}
	curBody, _ := w.store.Body(slug)
	in := skills.SkillInput{
		Name:        cur.Name,
		Description: cur.Description,
		WhenToUse:   cur.WhenToUse,
		Icon:        cur.Icon,
		Color:       cur.Color,
		Group:       cur.Group,
		Shared:      cur.Shared,
		Body:        curBody,
	}
	if name != nil {
		in.Name = *name
	}
	if description != nil {
		in.Description = *description
	}
	if whenToUse != nil {
		in.WhenToUse = *whenToUse
	}
	if group != nil {
		in.Group = *group
	}
	if body != nil {
		in.Body = *body
	}
	if shared != nil {
		in.Shared = *shared
	}
	_, err := w.store.Update(slug, in)
	return err
}

func (w agentSkillWriter) DeleteSkill(slug string) error {
	if err := w.store.Delete(slug); err != nil {
		return err
	}
	// Drop the now-deleted slug from any agent that referenced it so no agent
	// keeps a dangling skill reference.
	if w.db != nil {
		_, _ = w.db.RemoveSkillFromAgents(context.Background(), slug)
	}
	return nil
}

// SetScheduleReloader wires the scheduler's Reload so self-management schedule
// tools take effect immediately. Called by the workspace manager after the
// scheduler is constructed.
func (r *Runtime) SetScheduleReloader(fn func(context.Context) error) { r.reloadSched = fn }

// SetScheduleRunner wires the scheduler's RunNow so the run_schedule
// self-management tool can fire a schedule on demand. Called by the workspace
// manager after the scheduler is constructed. Nil leaves run_schedule a no-op.
func (r *Runtime) SetScheduleRunner(fn func(context.Context, string) error) { r.runSched = fn }

// runScheduleNow fires a schedule immediately (nil-safe; errors when unwired).
func (r *Runtime) runScheduleNow(ctx context.Context, id string) error {
	if r.runSched == nil {
		return fmt.Errorf("scheduler not available")
	}
	return r.runSched(ctx, id)
}

// SetTurnHook wires a single callback invoked when an agent turn finishes,
// replacing any hooks added so far. Kept for callers that want exactly one hook;
// AddTurnHook is preferred when several observers (automations + coordination)
// must all see turn completions.
func (r *Runtime) SetTurnHook(fn func(context.Context, TurnFinished)) {
	r.turnHooksMu.Lock()
	defer r.turnHooksMu.Unlock()
	if fn == nil {
		r.turnHooks = nil
		return
	}
	r.turnHooks = []func(context.Context, TurnFinished){fn}
}

// AddTurnHook appends a turn-completion observer. All hooks fire (each on its own
// detached goroutine) for every finished turn, so the AutomationEngine and the
// CoordinationEngine can both react without one shadowing the other.
func (r *Runtime) AddTurnHook(fn func(context.Context, TurnFinished)) {
	if fn == nil {
		return
	}
	r.turnHooksMu.Lock()
	defer r.turnHooksMu.Unlock()
	r.turnHooks = append(r.turnHooks, fn)
}

// AddFailedTurnHook appends a FAILED-turn observer. Kept separate from the
// success hooks: coordination must never receive an error turn as a worker
// result, but the automation engine must — a repair automation watching an
// error-class tag ("stuck"/"error") has to fire on the very turn that failed,
// not wait for a success that may never come (the stuck gate refuses further
// autonomous turns). Wired by the workspace manager next to SetTurnHook.
func (r *Runtime) AddFailedTurnHook(fn func(context.Context, TurnFinished)) {
	if fn == nil {
		return
	}
	r.turnHooksMu.Lock()
	defer r.turnHooksMu.Unlock()
	r.failedTurnHooks = append(r.failedTurnHooks, fn)
}

// FireTurnFailed dispatches a turn-FAILURE signal to the failed-turn hooks
// (detached, like FireTurnFinished). output carries the turn's error text so an
// automation's {{result}} placeholder renders the failure reason.
func (r *Runtime) FireTurnFailed(sessionID, agentID, output string) {
	if sessionID == "" {
		return
	}
	r.turnHooksMu.RLock()
	hooks := r.failedTurnHooks
	r.turnHooksMu.RUnlock()
	tf := TurnFinished{SessionID: sessionID, AgentID: agentID, Output: output}
	for _, fn := range hooks {
		fn := fn
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), spawnTimeout)
			defer cancel()
			fn(ctx, tf)
		}()
	}
}

// FireTurnFinished dispatches a turn-completion signal to every wired hook, each
// on its own DETACHED goroutine so a hook never blocks or cancels with the
// finishing turn. Safe to call from any completion path (chat/spawn/schedule/
// wake). No-op when no hook is wired or when sessionID is empty.
func (r *Runtime) FireTurnFinished(sessionID, agentID, output string) {
	if sessionID == "" {
		return
	}
	r.turnHooksMu.RLock()
	hooks := r.turnHooks
	r.turnHooksMu.RUnlock()
	tf := TurnFinished{SessionID: sessionID, AgentID: agentID, Output: output}
	for _, fn := range hooks {
		fn := fn
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), spawnTimeout)
			defer cancel()
			fn(ctx, tf)
		}()
	}
}

// SetSettingsBridge wires the application-wide settings store + live-apply hook
// so the get_settings / update_settings self-management tools become available.
// Called by the workspace manager once the api server (which owns the apply
// hook) exists. Nil leaves those tools off.
func (r *Runtime) SetSettingsBridge(b tools.SettingsBridge) { r.settingsBridge = b }

// SetWorkspaceBridge wires the cross-workspace management bridge so the
// list/create/rename/delete_workspace self-management tools become available.
// Called by the workspace manager once the api server exists. Nil leaves those
// tools off.
func (r *Runtime) SetWorkspaceBridge(b tools.WorkspaceBridge) { r.workspaceBridge = b }

// reloadSchedules re-reads schedules into the cron scheduler (nil-safe).
func (r *Runtime) reloadSchedules(ctx context.Context) error {
	if r.reloadSched == nil {
		return nil
	}
	return r.reloadSched(ctx)
}

// Wake delay bounds (seconds): a wake fires no sooner than this many seconds and
// no later than the cap, mirroring the scheduler's own clamp.
const (
	MinWakeDelaySec = 5
	MaxWakeDelaySec = 3600
)

// ScheduleWake arms a one-shot self-wake: after delaySeconds the agent is
// re-invoked with prompt inside sessionID (the originating chat session), so the
// conversation continues on its own. It persists a one-shot schedule and reloads
// the scheduler to arm the timer. Returns a short confirmation for the tool.
// reason is the agent's stated purpose (stored for context, surfaced in events).
func (r *Runtime) ScheduleWake(ctx context.Context, sessionID, agentID, prompt, reason string, delaySeconds int) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("schedule_wake is only available during an interactive chat turn")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("prompt is required (what to do when you wake)")
	}
	if delaySeconds < MinWakeDelaySec {
		delaySeconds = MinWakeDelaySec
	}
	if delaySeconds > MaxWakeDelaySec {
		delaySeconds = MaxWakeDelaySec
	}
	fireAt := time.Now().Add(time.Duration(delaySeconds) * time.Second).Unix()
	sc, err := r.db.CreateSchedule(ctx, db.Schedule{
		AgentID:   agentID,
		Prompt:    prompt,
		Reason:    reason,
		SessionID: sessionID,
		OneShot:   true,
		FireAt:    fireAt,
		Enabled:   true,
		CreatedBy: agentID,
	})
	if err != nil {
		return "", fmt.Errorf("schedule wake: %w", err)
	}
	if err := r.reloadSchedules(ctx); err != nil {
		return "", fmt.Errorf("wake saved (%s) but arming failed: %w", sc.ID, err)
	}
	// Tell an open session screen that the turn ended into a WAITING state (not a
	// finished one): phase=armed raises a "waiting to auto-resume" banner with the
	// reason and a Cancel control, so the chat no longer looks idle while the wake
	// timer counts down. emitWakePhase carries reason + fireAt for the UI.
	r.emitWakePhase(sessionID, "armed", reason, fireAt, "⏰ Otomatik uyandırma kuruldu")
	return fmt.Sprintf("Wake armed: in %ds I will continue this conversation on my own. Nothing more to do this turn.", delaySeconds), nil
}

// emitWakePhase publishes a "chat"-typed wake lifecycle event for a session. The
// frontend keys off target.phase: "armed" raises the waiting banner (with reason
// + fireAt), "cancelled" clears it. The fire path (scheduler) emits start/done.
func (r *Runtime) emitWakePhase(sessionID, phase, reason string, fireAt int64, title string) {
	target := map[string]string{"view": "chat", "sessionId": sessionID, "phase": phase}
	if reason != "" {
		target["reason"] = reason
	}
	if fireAt > 0 {
		target["fireAt"] = strconv.FormatInt(fireAt, 10)
	}
	r.publish(events.Event{
		Type:   "chat",
		Level:  "info",
		Title:  title,
		Body:   reason,
		Target: target,
	})
}

// CancelWake disarms any pending one-shot self-wake armed for sessionID (the user
// pressed "Durdur" on the waiting banner). It deletes the matching one-shot
// schedule rows, re-arms the scheduler so their timers are cancelled, and emits a
// phase=cancelled event so the open screen clears the waiting banner. Returns the
// number of wakes cancelled (0 when none were pending).
func (r *Runtime) CancelWake(ctx context.Context, sessionID string) (int, error) {
	if sessionID == "" {
		return 0, fmt.Errorf("sessionId is required")
	}
	schedules, err := r.db.ListEnabledSchedules(ctx)
	if err != nil {
		return 0, err
	}
	cancelled := 0
	for _, sc := range schedules {
		if !sc.OneShot || sc.SessionID != sessionID {
			continue
		}
		if err := r.db.DeleteSchedule(ctx, sc.ID); err != nil {
			return cancelled, fmt.Errorf("cancel wake %s: %w", sc.ID, err)
		}
		cancelled++
	}
	if cancelled == 0 {
		return 0, nil
	}
	if err := r.reloadSchedules(ctx); err != nil {
		return cancelled, fmt.Errorf("wake cancelled but re-arm failed: %w", err)
	}
	r.emitWakePhase(sessionID, "cancelled", "", 0, "⏰ Otomatik uyandırma iptal edildi")
	return cancelled, nil
}

// publish stamps the workspace identity onto an event and pushes it to the bus.
func (r *Runtime) publish(e events.Event) {
	e.WorkspaceID = r.wsID
	e.WorkspaceName = r.wsName
	r.bus.Publish(e) // nil-safe
}

// Emit publishes an event from outside the agent package (e.g. the API layer)
// with this workspace's identity stamped on it.
func (r *Runtime) Emit(e events.Event) { r.publish(e) }

// agentName resolves an agent's display name for event text, falling back to
// the id when the lookup fails.
func (r *Runtime) agentName(id string) string {
	if a, err := r.db.GetAgent(context.Background(), id); err == nil && a.Name != "" {
		return a.Name
	}
	return id
}

// BuildSystemPrompt composes the agent's persona from soul + identity plus the
// goal-usage hint. Exported as the SINGLE persona assembler: the chat/preview
// path (api package) uses it too, so the two paths can never drift apart.
func BuildSystemPrompt(a db.Agent) string {
	out := ""
	if a.Soul != "" {
		out = a.Soul
	}
	if a.Identity != "" {
		if out != "" {
			out += "\n\n"
		}
		out += a.Identity
	}
	// One-line nudge to use the session's north-star goal, in the cached static
	// prefix for every turn (chat and autonomous/flow both go through here).
	if out != "" {
		out += "\n\n"
	}
	out += GoalUsageHint
	return out
}

// GoalUsageHint is a single cached-prefix line teaching proactive goal use. The
// tools are always available (eager); this nudges the agent to actually reach for
// them on substantial work. Full guidance lives in the tionswarm-guide skill.
// Appended by BuildSystemPrompt (the single persona assembler for all paths).
const GoalUsageHint = "For substantial multi-turn work, set a durable objective with `set_session_goal` " +
	"(one north star, not a checklist) and `complete_goal` when it is met; keep replies aligned with the session's active goal."

// EnvironmentContextBlock renders a one-line machine-environment marker (OS,
// arch, native shell) so the agent writes shell commands in the correct syntax
// instead of guessing — on Windows the shell tool is PowerShell, on Unix it is
// Bash (NewShellRunner picks the same identity). Mirrors the external agent project's
// <environment> marker, trimmed to the one field that actually changes agent
// behaviour (shell). Exported so both the chat path (api.composeTurnRequest) and
// the headless path (autonomousSystemPrompt) inject the identical line. It rides
// the volatile dynamic suffix, so it never disturbs the cached static prefix.
func EnvironmentContextBlock() string {
	shell := "Bash"
	if runtime.GOOS == "windows" {
		shell = "PowerShell"
	}
	return fmt.Sprintf("<environment os=%q arch=%q shell=%q /> — write shell commands in %s syntax for this machine.",
		runtime.GOOS, runtime.GOARCH, shell, shell)
}

// ShellToolsContextBlock advertises the shell execution tools (Bash / PowerShell)
// ONLY when this workspace's shell gate is on (Tunables.ShellEnabled) AND a
// backing interpreter is present — mirroring buildRegistry's registration
// conditions exactly (same resolvers via tools.ShellToolNames). When the gate is
// off the block is empty, so the workspace instructions never promise a shell tool
// that was never registered — the mismatch that made the model emit a bare
// `PowerShell` call and hit "No such tool available: PowerShell … not enabled in
// this context". It rides the VOLATILE dynamic suffix on both the chat
// (composeTurnRequest) and headless (autonomousDynamicSuffix) paths because the
// gate can toggle mid-session; the file tools (Read/Write/Edit/LS/Glob/Grep) stay
// the always-on core named in the static instructions.
func (r *Runtime) ShellToolsContextBlock() string {
	if !r.tun.ShellEnabled() {
		return ""
	}
	names := tools.ShellToolNames()
	if len(names) == 0 {
		return ""
	}
	noun, verb := "tool", "runs"
	if len(names) > 1 {
		noun, verb = "tools", "run"
	}
	return "Shell execution is ENABLED for this session: the " + strings.Join(names, " / ") + " " + noun +
		" " + verb + " host commands — not confined to the working directory (absolute paths and `..` allowed), " +
		"with the permission mode as the safety layer. Call " + strings.Join(names, " / ") + " by that exact name."
}

// systemPrompt builds an agent's static system prefix: its soul+identity persona
// followed by this workspace's instructions (when set). Both are stable, so they
// belong in the cached static prefix rather than the volatile dynamic suffix.
func (r *Runtime) systemPrompt(a db.Agent) string {
	out := BuildSystemPrompt(a)
	if p := r.instructions.Load(); p != nil {
		if ins := strings.TrimSpace(*p); ins != "" {
			if out != "" {
				out += "\n\n"
			}
			out += "# Workspace Instructions\n" + ins
		}
	}
	return out
}

// autonomousSystemPrompt is systemPrompt plus the agent's Available Skills block,
// for headless runs (scheduler/spawn/flow). Chat turns add the catalog
// in composeTurnRequest; the autonomous entry points (which build their own
// request) had no catalog, so a scheduled agent never learned its skills. Adding
// it here — together with the autonomous Interaction use_skill bridge — gives
// headless runs the same skill access chat agents have.
func (r *Runtime) autonomousSystemPrompt(ctx context.Context, a db.Agent) string {
	out := r.systemPrompt(a)
	if sb := r.SkillsCatalogBlockForAgent(a); sb != "" {
		out = strings.TrimSpace(out + "\n\n" + sb)
	}
	// Advertise optional external-tool capabilities (e.g. codebase-memory) present
	// in this workspace so a headless turn reaches for them too, WITH the session's
	// cwd-derived project id (ctx carries the session id on scheduler/spawn/flow
	// paths). Presence is stable, so it rides the cached static prefix. Shares ONE
	// source with the chat path (api.composeTurnRequest).
	cwd := r.sessionCwd(ctx)
	if cb := r.CapabilityContext(ctx, cwd); cb != "" {
		out = strings.TrimSpace(out + "\n\n" + cb)
	}
	// Best-effort: ensure the session's repo is indexed in this workspace's isolated
	// store (guarded once per cwd per process; no-op without a cwd or an enabled
	// codebase-memory server).
	r.EnsureCodebaseIndexed(ctx, cwd)
	// Boot/verification sequence (Anthropic long-running-agent harness discipline):
	// a headless turn starts with a fresh context, so nudge it through the fixed
	// orient → recall → select-one → verify-baseline → work → close-the-loop routine
	// before acting. We inject only a pointer to keep the cached prefix small; the
	// full recipe lives in the tionswarm-autonomous-ops skill.
	if r.tun.AutonomousBootSeq() {
		out = strings.TrimSpace(out + "\n\n" + autonomousBootReminder)
	}
	// Machine-environment marker (OS/arch/shell) so a headless turn writes shell
	// commands in the right syntax. Its bytes never change within a process, so
	// it is safe inside the cached static prefix. The VOLATILE pieces (turn-start
	// clock, session goal) deliberately live in autonomousDynamicSuffix — putting
	// them here would change the prefix bytes every turn and defeat prompt
	// caching for every headless run.
	out = strings.TrimSpace(out + "\n\n" + EnvironmentContextBlock())
	return out
}

// DateTimeContextBlock renders the turn-start clock line. Exported so the chat
// path (api.composeTurnRequest) and the headless dynamic suffix share ONE
// wording. It is volatile by nature, so it must ride SystemDynamic — never the
// cached static prefix.
func DateTimeContextBlock() string {
	return "Current date and time (captured at the start of this turn; seconds-precise, does not tick mid-turn): " +
		time.Now().Format("Monday, 2006-01-02 15:04:05 (-07:00)")
}

// autonomousDynamicSuffix builds the VOLATILE system suffix for headless turns
// (scheduler/spawn/flow/subagent): the turn-start clock plus the session's
// active goal (if any). Chat turns assemble the same pieces in
// composeTurnRequest; keeping them out of autonomousSystemPrompt keeps the
// static prefix byte-stable across turns so cache-capable providers reuse it.
func (r *Runtime) autonomousDynamicSuffix(ctx context.Context) string {
	out := DateTimeContextBlock()
	if g := r.autonomousGoalBlock(ctx); g != "" {
		out += "\n\n" + g
	}
	// Failure lessons (hata→ders döngüsü): the newest distilled lessons ride
	// every headless turn so a fresh context does not repeat known failures.
	// The turn's own agent (resolved via the stamped session) ranks first.
	agentID := ""
	if sid := SessionIDFrom(ctx); sid != "" {
		if sess, err := r.db.GetSession(ctx, sid); err == nil {
			agentID = sess.AgentID
		}
	}
	if lb := r.LessonsContextBlock(ctx, agentID); lb != "" {
		out += "\n\n" + lb
	}
	// Shell tools (Bash/PowerShell) are gated; advertise them only when actually
	// registered (gate on + backing shell present). Volatile → dynamic suffix,
	// mirroring the chat path, since the gate can toggle mid-session.
	if sh := r.ShellToolsContextBlock(); sh != "" {
		out += "\n\n" + sh
	}
	return out
}

// autonomousBootReminder frames every headless turn (schedule/spawn/flow/
// subagent). Deliberately stated as GOALS AND BOUNDARIES rather than a numbered
// step recipe: current-generation models (Fable 5 class) follow intent well and
// over-prescriptive scaffolding measurably reduces their output quality. Three
// concerns, per Anthropic's long-running-agent guidance: (1) autonomy — no user
// is watching, act instead of asking or ending on a plan; (2) grounded progress
// — claims must be backed by a tool result from this session; (3) durable
// closure — record what changed so the next fresh context can pick it up. The
// detailed recipe stays in the tionswarm-autonomous-ops skill.
const autonomousBootReminder = "# Autonomous operation\n" +
	"This is a headless turn with a fresh context; no user is watching and none can answer questions, " +
	"so do not ask permission and do not end the turn with a plan or a promise — for reversible actions " +
	"that follow from the task, act. Orient yourself before changing anything (working directory, git state, " +
	"the persisted progress file, open tasks) and verify the baseline is green before building on it; " +
	"fix a broken baseline first. Work on ONE piece of work per turn, done properly, rather than several half-done. " +
	"Before reporting progress, check each claim against a tool result from this session — report only what you can " +
	"point to evidence for, and say explicitly when something is not yet verified. Close the loop when finished: " +
	"commit/record what changed and append a progress note (never overwrite a prior note). " +
	"Playbook when needed: use_skill \"tionswarm-autonomous-ops\"."

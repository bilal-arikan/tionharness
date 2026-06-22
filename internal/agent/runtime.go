// Package agent implements SwarmGo's multi-agent ("swarm") runtime: it owns each
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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/logbuf"
	"github.com/bilal/swarmgo/internal/market"
	"github.com/bilal/swarmgo/internal/memory"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/secrets"
	"github.com/bilal/swarmgo/internal/skills"
	"github.com/bilal/swarmgo/internal/tools"
)

// Runtime owns the lifecycle of all autonomous agent workers.
type Runtime struct {
	db        *db.DB
	providers *providers.Registry
	mem       *memory.Store
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

	// reflecting guards against concurrent auto-reflects for the same agent: a
	// burst of journaled turns must not spawn overlapping dream cycles. Keyed by
	// agent id; presence means a reflection is in flight.
	reflecting sync.Map

	// activeSessions tracks sessions currently executing an autonomous invoke
	// (schedule / spawn). Keyed by session id; value is struct{}.
	// Used by the executions feed to show a live "running" indicator for
	// autonomous runs that aren't chat-streaming turns.
	activeSessions sync.Map

	// spawnActive counts the spawned sessions currently running their background
	// turn — the fire-and-forget concurrency guard (capped by SpawnMaxConcurrent).
	spawnActive atomic.Int64
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

// NewRuntime constructs the runtime. tun carries the process-wide tunables
// (autonomy pause, title-model override) shared across all workspace runtimes.
// workDir is the workspace sandbox root for built-in filesystem/shell tools.
// bus + wsID/wsName let autonomous events be published with workspace context
// (bus may be nil, in which case publishing is a no-op). vault is this
// workspace's secret store handed to the secret_* tools (may be nil).
func NewRuntime(database *db.DB, registry *providers.Registry, tun *Tunables, workDir string, vault *secrets.Vault, bus *events.Bus, wsID, wsName string, logs *logbuf.Buffer, logger *slog.Logger) *Runtime {
	// Seed the shipped default skills into the global dir (idempotent, never
	// overwrites) so every workspace inherits the SwarmGo guide skills.
	_ = skills.EnsureDefaults(globalSkillsDir())
	// Seed the bundled marketplace starter packs into the global market dir
	// (idempotent, never overwrites) so every workspace can browse them.
	_ = market.EnsureDefaults(marketGlobalDir())
	return &Runtime{
		db:        database,
		providers: registry,
		mem:       memory.New(database),
		tun:       tun,
		workDir:   workDir,
		vault:     vault,
		bus:       bus,
		wsID:      wsID,
		wsName:    wsName,
		logs:      logs,
		logger:    logger,
		skills:    skills.New(globalSkillsDir(), workspaceSkillsDir(workDir)),
		market:    market.New(marketGlobalDir(), workspaceMarketDir(workDir)),
	}
}

// globalSkillsDir is SwarmGo's data-dir-level global skills directory
// (<DataDir>/skills, default ~/.swarmgo/skills). Deliberately under SwarmGo's
// OWN data dir — not the cross-tool ~/.agents/skills convention — so SwarmGo's
// global skills stay isolated from other agent tools that share that directory.
// Honors SWARMGO_DATA_DIR so a custom data dir is respected (mirrors config).
func globalSkillsDir() string {
	if d := os.Getenv("SWARMGO_DATA_DIR"); d != "" {
		return filepath.Join(d, "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".swarmgo", "skills")
}

// workspaceSkillsDir is this workspace's skills directory (<workspace>/skills),
// a sibling of store/, config/ and workspace/. Empty when workDir is unknown.
func workspaceSkillsDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "skills")
}

// Skills returns this runtime's skill store (never nil after construction).
func (r *Runtime) Skills() *skills.Store { return r.skills }

// Market returns this runtime's marketplace store (never nil after construction).
func (r *Runtime) Market() *market.Store { return r.market }

// WorkspaceSkillsDir is the workspace's skills directory, where the market
// installs skill packs. Exposed so the API layer can pass it to InstallSkill.
func (r *Runtime) WorkspaceSkillsDir() string { return workspaceSkillsDir(r.workDir) }

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

// NewShellRunner returns a closure that runs a shell command through the
// workspace-sandboxed shell tool (PowerShell on Windows, /bin/sh elsewhere), for
// the claude-cli Interaction MCP bridge — so a CLI agent runs commands through
// SwarmGo's own shell (sandboxed, bounded, permission/hook-gated) instead of the
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
	// path. Unconfined, like an interactive turn.
	return func(ctx context.Context, args json.RawMessage) (string, error) {
		return tools.NewShellTool(tools.NewSandbox(r.effectiveWorkDir(ctx))).Call(ctx, args)
	}
}

// marketGlobalDir is SwarmGo's data-dir-level global market directory
// (<DataDir>/market, default ~/.swarmgo/market). Mirrors globalSkillsDir.
func marketGlobalDir() string {
	if d := os.Getenv("SWARMGO_DATA_DIR"); d != "" {
		return filepath.Join(d, "market")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".swarmgo", "market")
}

// workspaceMarketDir is this workspace's market directory (<workspace>/market),
// a sibling of store/, skills/ and config/. Empty when workDir is unknown.
func workspaceMarketDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "market")
}

// SkillsCatalogBlockForAgent renders the Available Skills system-prompt section
// an agent sees: its assigned skills (in the agent's chosen order) plus every
// shared (on-demand) skill. Returns "" when neither exists. Skills are a shared
// library; agents pick from it — they never own skills.
func (r *Runtime) SkillsCatalogBlockForAgent(agent db.Agent) string {
	if r.skills == nil {
		return ""
	}
	return r.skills.CatalogBlockForAgent(agent.Skills)
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
	defs := reg.BridgeableDefs(r.toolFilter(ctx, agent))
	call := func(ctx context.Context, name string, args json.RawMessage) (string, error) {
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

// agentSkillWriter adapts *skills.Store to the tools.SkillWriter interface so the
// create_skill / delete_skill self-management tools can author workspace skills
// without the tools package importing the skills package. db (optional) lets
// DeleteSkill strip the removed slug from every agent's skill selection.
type agentSkillWriter struct {
	store *skills.Store
	db    *db.DB
}

func (w agentSkillWriter) CreateSkill(slug, name, description, whenToUse, body string, shared bool) error {
	_, err := w.store.Create(slug, skills.SkillInput{
		Name:        name,
		Description: description,
		WhenToUse:   whenToUse,
		Body:        body,
		Shared:      shared,
	})
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

// Memory exposes the runtime's memory store for handlers in the same workspace.
func (r *Runtime) Memory() *memory.Store { return r.mem }

// buildSystemPrompt composes the agent's persona from soul + identity.
func buildSystemPrompt(a db.Agent) string {
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
	return out
}

// systemPrompt builds an agent's static system prefix: its soul+identity persona
// followed by this workspace's instructions (when set). Both are stable, so they
// belong in the cached static prefix rather than the volatile dynamic suffix.
func (r *Runtime) systemPrompt(a db.Agent) string {
	out := buildSystemPrompt(a)
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
func (r *Runtime) autonomousSystemPrompt(a db.Agent) string {
	out := r.systemPrompt(a)
	if sb := r.SkillsCatalogBlockForAgent(a); sb != "" {
		out = strings.TrimSpace(out + "\n\n" + sb)
	}
	return out
}

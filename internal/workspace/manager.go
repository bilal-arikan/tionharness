// Package workspace provides fully-isolated workspaces. Each workspace owns its
// own file-based store directory and its own agent runtime, so content is
// completely independent between workspaces — switching one never leaks into another.
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
	flowpkg "github.com/bilal-arikan/tionharness/internal/flows"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/secrets"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/worktree"
)

// Meta is the persisted descriptor of a workspace (no live handles). Path, when
// set, is the workspace's data directory chosen by the user; empty means the
// default location under the manager root.
type Meta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	Path      string `json:"path,omitempty"`
	// CreatedBy is the id of the agent that created this workspace via a
	// self-management tool. Empty means the user created it (in the UI). Only
	// agent-created workspaces (CreatedBy != "") may be deleted by an agent.
	CreatedBy string `json:"createdBy,omitempty"`
}

// DegradedWorkspace is a registered workspace that could not be opened. It keeps
// the registry metadata (so persist() writes the entry back) plus the reason the
// open failed, which is the only thing that lets the user tell a broken workspace
// from a healthy one without reading the server log.
type DegradedWorkspace struct {
	Meta
	// Reason is the open error's message, rendered at the time it happened.
	Reason string
}

// ListEntry is one row of the registry as the UI sees it: a workspace's metadata
// plus whether it is live or degraded. Degraded rows carry no live handles, so a
// consumer must not expect Get(entry.ID) to succeed for them.
type ListEntry struct {
	Meta
	Degraded bool
	Reason   string
}

// Workspace bundles a workspace's live database, runtime and scheduler.
type Workspace struct {
	Meta
	DB          *db.DB
	Runtime     *agent.Runtime
	Scheduler   *agent.Scheduler
	InsightCron *agent.InsightCron
	Secrets     *secrets.Vault
	DataDir     string

	settings settingsHolder // per-workspace overrides (ws-settings.json)
}

// SandboxRoot is the workspace's file sandbox (<DataDir>/workspace): the root
// that uploads, artifacts and the files API resolve their relative paths
// against. Named so the several call sites that used to spell out
// filepath.Join(DataDir, "workspace") agree by construction.
func (w *Workspace) SandboxRoot() string { return filepath.Join(w.DataDir, "workspace") }

// Manager owns all workspaces and persists their registry.
type Manager struct {
	rootDir  string
	registry *providers.Registry
	tun      *agent.Tunables
	cipher   secrets.Cipher
	bus      *events.Bus
	logs     *logbuf.Buffer
	logger   *slog.Logger

	mu         sync.RWMutex
	workspaces map[string]*Workspace
	order      []string // creation order (first = default)

	// bg tracks the detached goroutines the store hooks dispatch (activity-inbox
	// drains, automation and card-worktree reactions). They outlive the call that
	// started them and touch the workspace's store directory, so Close waits for
	// them: without that, a caller that closes the manager and then removes the
	// tree — the app on shutdown, and every test using t.TempDir — races a drain
	// still writing under store/activity-inbox and the removal fails.
	// bgClosed (under bgMu) makes the wait final: no goroutine may be added once
	// Close has begun draining.
	bgMu     sync.Mutex
	bgClosed bool
	bg       sync.WaitGroup

	// degraded holds every workspace that is REGISTERED but could not be opened (a
	// corrupt store file, a missing/unreadable data directory). It is not live — it
	// has no DB or runtime — but persist() writes it back to workspaces.json all
	// the same. Dropping it would mean the next Create/Delete rewrites the registry
	// without it and the workspace disappears from the app forever even though all
	// of its data is still on disk. Guarded by mu.
	degraded []DegradedWorkspace

	// pendingRemoval holds the data directories that a Delete/deleteDegraded is
	// erasing RIGHT NOW. The registry record is already gone at that point, but
	// os.RemoveAll deliberately runs outside m.mu (slow, destructive IO), so
	// without this list Attach could adopt the very folder that is about to be
	// wiped and the removal would take the freshly attached workspace's data with
	// it. A path is added in the same hold that drops the registry record and
	// removed once the deletion has returned. Guarded by mu.
	pendingRemoval []string

	// wsCounter is the monotonic sequence behind human-readable workspace ids
	// ("WS1", "WS2"), persisted to ws-counter.json so a number is never reused
	// across deletions or restarts. Guarded by mu.
	wsCounter int64

	// globalModelRes is the installation-wide model-resolution store handed to
	// every workspace DB as it opens. nil when the document on disk could not be
	// read, in which case each workspace keeps only its own observations.
	globalModelRes *db.GlobalModelResolutions

	// settingsBridge is the application-wide settings store + live-apply hook,
	// wired in after the api server is constructed. Stored so it can be applied
	// both to existing runtimes (via SetSettingsBridge) and to any workspace
	// opened later. nil until wired.
	settingsBridge tools.SettingsBridge

	// workspaceBridge backs the workspace self-management tools (list/create/
	// rename/delete_workspace). Like settingsBridge it is wired in after the api
	// server exists and distributed to every runtime (existing + later-opened).
	// nil until wired.
	workspaceBridge tools.WorkspaceBridge

	// autoInteractFactory builds the headless Interaction MCP setup for a runtime
	// (so scheduler/spawn CLI agents reach use_skill/shell/self-manage).
	// Wired in after the api server exists; applied to existing + later-opened
	// runtimes. nil until wired.
	autoInteractFactory func(*agent.Runtime) agent.AutonomousInteraction

	// deadToolActivator activates an on-demand Interaction MCP tool the model
	// called before turning it on, so the CLI's "No such tool available" dead end
	// self-repairs. Token-addressed and therefore workspace-independent, so a single
	// value is shared by every runtime (unlike the per-runtime factories above).
	// Wired in after the api server exists. nil until wired.
	deadToolActivator agent.DeadToolActivator

	// extActiveProbe reports a workspace's in-flight INTERACTIVE chat sessions.
	// The api server owns that registry; the runtime needs it to answer "is this
	// agent busy" (AgentBusy) for the delete guard. Wired in after the api server
	// exists; applied to existing + later-opened runtimes. nil until wired.
	extActiveProbe func(workspaceID string) []string

	// wakeTurnFactory builds the history-aware self-wake turn runner for a runtime
	// (so schedule_wake continues with the full conversation, not just the wake
	// prompt). Wired in after the api server exists; applied to existing +
	// later-opened runtimes. nil until wired.
	wakeTurnFactory func(*agent.Runtime) agent.WakeTurnFunc
}

// DataDir returns the application-wide data root.
func (m *Manager) DataDir() string { return m.rootDir }

// SetAutonomousInteraction wires the headless Interaction MCP factory into every
// existing workspace runtime and remembers it for workspaces opened later. The
// api server calls this once at startup (it owns the run registry + endpoint).
func (m *Manager) SetAutonomousInteraction(factory func(*agent.Runtime) agent.AutonomousInteraction) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoInteractFactory = factory
	for _, ws := range m.workspaces {
		ws.Runtime.SetAutonomousInteraction(factory(ws.Runtime))
	}
}

// SetDeadToolActivator wires the on-demand tool activator behind the dead-tool
// repair into every existing workspace runtime and remembers it for workspaces
// opened later. The api server calls this once at startup (it owns the run
// registry and the per-session activation state).
func (m *Manager) SetDeadToolActivator(fn agent.DeadToolActivator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deadToolActivator = fn
	for _, ws := range m.workspaces {
		ws.Runtime.SetDeadToolActivator(fn)
	}
}

// SetExternalActiveSessions wires the api server's chat-run registry into every
// existing workspace runtime and remembers it for workspaces opened later, so
// Runtime.AgentBusy sees INTERACTIVE turns too — not just the autonomous ones it
// tracks itself. The probe is bound per workspace (the registry is server-wide
// and must be scoped).
func (m *Manager) SetExternalActiveSessions(probe func(workspaceID string) []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.extActiveProbe = probe
	for _, ws := range m.workspaces {
		id := ws.ID
		ws.Runtime.SetExternalActiveSessions(func() []string { return probe(id) })
	}
}

// SetWakeTurnRunner wires the history-aware self-wake turn runner factory into
// every existing workspace runtime and remembers it for workspaces opened later.
// The api server calls this once at startup (it owns chat-turn composition).
func (m *Manager) SetWakeTurnRunner(factory func(*agent.Runtime) agent.WakeTurnFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.wakeTurnFactory = factory
	for _, ws := range m.workspaces {
		ws.Runtime.SetWakeTurnRunner(factory(ws.Runtime))
	}
}

// SetSettingsBridge wires the application-wide settings bridge into every
// existing workspace runtime and remembers it for workspaces opened later. The
// api server calls this once at startup (it owns the live-apply hook).
func (m *Manager) SetSettingsBridge(b tools.SettingsBridge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settingsBridge = b
	for _, ws := range m.workspaces {
		ws.Runtime.SetSettingsBridge(b)
	}
}

// SetWorkspaceBridge wires the cross-workspace management bridge into every
// existing workspace runtime and remembers it for workspaces opened later. The
// api server calls this once at startup (it owns the manager + seed/notify hooks).
func (m *Manager) SetWorkspaceBridge(b tools.WorkspaceBridge) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.workspaceBridge = b
	for _, ws := range m.workspaces {
		ws.Runtime.SetWorkspaceBridge(b)
	}
}

// NewManager loads the registry from disk, opens every workspace, and ensures
// at least one default workspace exists. tun is the shared process-wide
// tunables handed to every workspace runtime; bus is the process-wide event bus
// each runtime publishes autonomous notifications to.
func NewManager(rootDir string, registry *providers.Registry, tun *agent.Tunables, cipher secrets.Cipher, bus *events.Bus, logs *logbuf.Buffer, logger *slog.Logger) (*Manager, error) {
	m := &Manager{
		rootDir:    rootDir,
		registry:   registry,
		tun:        tun,
		cipher:     cipher,
		bus:        bus,
		logs:       logs,
		logger:     logger,
		workspaces: make(map[string]*Workspace),
	}

	// App-global model resolutions: shared by every workspace DB so an alias one
	// workspace resolved is nameable in all of them. A corrupt document is not
	// fatal — it only costs the cross-workspace fallback — but it is reported.
	if globalModelRes, err := db.OpenGlobalModelResolutions(rootDir); err != nil {
		logger.Warn("app-global model resolutions unreadable", "error", err)
	} else {
		m.globalModelRes = globalModelRes
	}

	metas, err := m.loadMetas()
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(metas))
	for _, meta := range metas {
		dir := meta.Path
		if dir == "" {
			dir = filepath.Join(rootDir, "workspaces", meta.ID)
		}
		roots = append(roots, dir)
	}
	if err := agent.MigrateSharedCLIHomes(rootDir, roots, logger); err != nil {
		return nil, err
	}

	// Restore the workspace id counter; guard against rewind by also taking the
	// max of any "WS<n>" id already on disk (so a stale/missing counter file can
	// never reissue a live id).
	m.wsCounter = m.loadWSCounter()
	for _, meta := range metas {
		if n, ok := parseWSID(meta.ID); ok && n > m.wsCounter {
			m.wsCounter = n
		}
	}

	for _, meta := range metas {
		if err := m.open(meta); err != nil {
			// Keep the registry entry (see Manager.degraded): an open failure is a
			// reason to report a workspace as broken, never a reason to delete it.
			logger.Error("failed to open workspace; keeping its registry entry", "id", meta.ID, "path", meta.Path, "error", err)
			m.markDegraded(meta, err)
			continue
		}
	}

	// First-run onboarding owns workspace creation: when none exist we deliberately
	// leave the manager EMPTY rather than seeding a default one. The web UI shows a
	// splash + "create workspace" popup on a fresh install; if the user dismisses it
	// without creating one, nothing is provisioned. With zero workspaces, ws(r) in
	// the API resolves to nil for workspace-scoped routes — the frontend gates those
	// behind an active workspace, and withWorkspace short-circuits any stray call
	// with a clean 409 ("no active workspace"), never dereferencing the nil.
	return m, nil
}

// formatLoadPhases renders the store's boot phase timings slowest-first as
// "name=ms" pairs, dropping sub-millisecond phases so the line stays readable.
// The total on its own never says WHICH loader to fix; this does.
func formatLoadPhases(phases map[string]int64) string {
	if len(phases) == 0 {
		return ""
	}
	type kv struct {
		name string
		ms   int64
	}
	list := make([]kv, 0, len(phases))
	for name, ms := range phases {
		if ms > 0 {
			list = append(list, kv{name, ms})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ms > list[j].ms })
	var b strings.Builder
	for i, e := range list {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%d", e.name, e.ms)
	}
	return b.String()
}

// open instantiates a workspace's DB + runtime and registers it in memory.
func (m *Manager) open(meta Meta) error {
	// A user-chosen Path overrides the default per-workspace location.
	dir := meta.Path
	if dir == "" {
		dir = filepath.Join(m.rootDir, "workspaces", meta.ID)
	}
	if err := os.MkdirAll(filepath.Join(dir, "workspace"), 0o755); err != nil {
		return err
	}

	// No per-workspace CLI config home is provisioned any more: claude-cli and
	// codex-cli run against the app-global homes (<dataDir>/claude-home,
	// <dataDir>/codex-home) unless the provider INSTANCE names its own configDir.
	// The legacy <workspace>/claude-home dirs are inert leftovers.

	storeDir := filepath.Join(dir, "store")
	storeOpenStart := time.Now()
	database, err := db.Open(storeDir)
	if err != nil {
		return err
	}
	database.SetGlobalModelResolutions(m.globalModelRes)
	// Seed the small core system-agent set for new workspaces, migrate renamed
	// roles, and backfill older workspaces. Existing customisations are preserved.
	if err := database.EnsureSystemAgents(context.Background(), agent.SystemAgentDefaults()...); err != nil {
		return fmt.Errorf("seed system agents: %w", err)
	}
	// Fill in agent rows written while an empty ThinkingLevel was still a legal
	// (but ambiguous) third state. Behaviour-preserving and idempotent — the next
	// boot finds nothing to do. Runs after the seeding above so freshly created
	// system agents are covered by the same pass.
	if migrated, err := database.BackfillThinkingLevels(context.Background()); err != nil {
		return fmt.Errorf("backfill thinking levels: %w", err)
	} else if migrated > 0 {
		m.logger.Info("thinking levels backfilled", "workspace", meta.ID, "agents", migrated)
	}
	// Boot cost of THIS workspace's store, attributed per workspace so a slow
	// startup points at the workspace responsible instead of a single total. It is
	// the regression metric for the message lazy-loading work: db.Open parses every
	// session's whole transcript today (db.load → loadSessions), so message_mb is
	// exactly the resident cost that work is meant to remove.
	storeStats := database.Stats()
	m.logger.Info("store opened",
		"workspace", meta.ID,
		"ms", time.Since(storeOpenStart).Milliseconds(),
		"sessions", storeStats.Sessions,
		"messages", storeStats.Messages,
		"message_mb", storeStats.MessageBytes>>20,
		"phases_ms", formatLoadPhases(storeStats.LoadPhaseMs))

	// Seed the built-in default flows into this workspace's store. Idempotent and
	// deletion-aware (a ledger keeps a user-removed default from coming back).
	// Running it here backfills every existing workspace on the next startup.
	if err := flowpkg.EnsureDefaultFlows(context.Background(), database, storeDir); err != nil {
		m.logger.Warn("seed default flows failed", "workspace", meta.ID, "error", err)
	}
	// Upgrade existing flows to the required start-node model (adds a start node
	// where missing). Idempotent; backfills every workspace on the next startup.
	if err := flowpkg.MigrateFlowsStartEnd(context.Background(), database); err != nil {
		m.logger.Warn("migrate flows to start-node model failed", "workspace", meta.ID, "error", err)
	}
	// Seed the built-in automations (board-driven execution: card→in_progress spawns
	// an agent, card→done archives; plus the insight-apply rule). Idempotent,
	// deletion-aware, and seeded DISABLED so activating one stays an explicit
	// per-workspace opt-in.
	if err := agent.EnsureDefaultAutomations(context.Background(), database, storeDir); err != nil {
		m.logger.Warn("seed default automations failed", "workspace", meta.ID, "error", err)
	}

	// Collapse the near-duplicate lessons written before signature-similarity
	// dedupe existed (the insight lens re-invented a slug per run, so one topic
	// occupied several rows and crowded the injected context). Idempotent, so
	// running it on every open is a no-op once a workspace is clean.
	if res, dErr := database.DedupeLessons(); dErr != nil {
		m.logger.Warn("lesson dedupe failed", "workspace", meta.ID, "error", dErr)
	} else if res.Merged > 0 {
		m.logger.Info("lessons deduped", "workspace", meta.ID, "before", res.Before, "after", res.After, "merged", res.Merged)
	}

	// Per-workspace secret vault (AES-GCM encrypted), shared by the secret_* tools.
	vault, err := secrets.Open(storeDir, m.cipher)
	if err != nil {
		return err
	}

	rt := agent.NewRuntime(database, m.registry, m.tun, filepath.Join(dir, "workspace"), m.rootDir, vault, m.bus, meta.ID, meta.Name, m.logs, m.logger)

	// Apply the settings bridge if it has already been wired (workspaces created
	// after startup); startup workspaces get it via SetSettingsBridge instead.
	if m.settingsBridge != nil {
		rt.SetSettingsBridge(m.settingsBridge)
	}
	if m.workspaceBridge != nil {
		rt.SetWorkspaceBridge(m.workspaceBridge)
	}
	if m.autoInteractFactory != nil {
		rt.SetAutonomousInteraction(m.autoInteractFactory(rt))
	}
	if m.deadToolActivator != nil {
		rt.SetDeadToolActivator(m.deadToolActivator)
	}
	if m.extActiveProbe != nil {
		probe, id := m.extActiveProbe, meta.ID
		rt.SetExternalActiveSessions(func() []string { return probe(id) })
	}
	if m.wakeTurnFactory != nil {
		rt.SetWakeTurnRunner(m.wakeTurnFactory(rt))
	}

	sched := agent.NewScheduler(database, rt, m.logger.With("component", "scheduler", "workspace", meta.ID))
	// Let self-management schedule tools reload the cron scheduler immediately.
	rt.SetScheduleReloader(sched.Reload)
	// Let the run_schedule tool fire a schedule on demand ("Run now").
	rt.SetScheduleRunner(sched.RunNow)
	if err := sched.Start(context.Background()); err != nil {
		m.logger.Warn("start scheduler failed", "workspace", meta.ID, "error", err)
	}

	// Insight auto-scan (retrospective scanning, _Docs/60 Faz 3): a dedicated cron
	// that calls RunInsightScan directly on the settings-driven schedule. Wire its
	// Reload so the settings endpoint can re-arm it, then start it.
	insightCron := agent.NewInsightCron(rt, m.logger.With("component", "insight-cron", "workspace", meta.ID))
	rt.SetInsightCronReloader(insightCron.Reload)
	if err := insightCron.Start(context.Background()); err != nil {
		m.logger.Warn("start insight cron failed", "workspace", meta.ID, "error", err)
	}

	// Tag-triggered automations: wire the event-driven engine as the runtime's turn
	// hook so a tagged session finishing a turn can spawn a follow-up (the loop).
	autoEngine := agent.NewAutomationEngine(database, rt, m.logger.With("component", "automation", "workspace", meta.ID))
	rt.SetTurnHook(autoEngine.OnTurnFinished)
	// Failed turns dispatch to the automation engine ONLY (not coordination):
	// repair automations watching error-class tags ("stuck"/"error") must fire
	// on the failing turn itself — a stuck session gets no more autonomous
	// successes to fire from.
	rt.AddFailedTurnHook(autoEngine.OnTurnFinished)
	// Token-triggered automations: every recorded provider call signals cumulative
	// spend so the engine can fire when a session/workspace crosses a threshold.
	rt.AddUsageHook(autoEngine.OnUsageRecorded)
	// Rota (F2): phase / trajectory_end automations and recipe watchers fire on
	// trajectory transitions, delivered off-path on the trajectory work queue.
	rt.SetTrajectoryTransitionHook(autoEngine.OnTrajectoryTransition)
	// Every message append is session activity regardless of author (user, agent,
	// worker, or system). Notify open clients so their session list picks up the
	// UpdatedAt written by AddMessage instead of waiting for a turn-done event.
	// Counter-triggered automations consume the same signal on a detached goroutine.
	// Workspace event stream (_Docs/77 R3): session lifecycle and trajectory
	// changes leave the store through these hooks (fired after the store's locks
	// are released) and land on the bus as structured ws:* events, which the API
	// bridges onto the ordered per-workspace stream.
	database.SetSessionHook(rt.OnSessionChange)
	database.SetTrajectoryHook(rt.OnTrajectoryChange)
	if err := database.SetActivityHook(func(sig db.ActivitySignal) error {
		if sig.EventID != "" {
			accepted, err := database.AcceptActivitySignal(sig)
			if err != nil {
				return err
			}
			if accepted {
				rt.Emit(events.Event{
					Type: events.TypeSession, Level: "info",
					Target: map[string]string{"sessionId": sig.SessionID, "op": "message_activity"},
				})
			}
			m.goBackground(func() {
				if err := autoEngine.DrainActivityInbox(context.Background()); err != nil {
					m.logger.Error("drain durable activity inbox failed", "workspace", meta.ID, "error", err)
				}
			})
			return nil
		}
		rt.Emit(events.Event{
			Type:   events.TypeSession,
			Level:  "info",
			Target: map[string]string{"sessionId": sig.SessionID, "op": "message_activity"},
		})
		m.goBackground(func() {
			autoEngine.OnActivityRecorded(context.Background(), agent.ActivityRecorded{
				SessionID:             sig.SessionID,
				MessageTotal:          sig.MessageTotal,
				MessageDelta:          sig.MessageDelta,
				ToolTotal:             sig.ToolTotal,
				ToolDelta:             sig.ToolDelta,
				WorkspaceMessageTotal: sig.WorkspaceMessageTotal,
				WorkspaceToolTotal:    sig.WorkspaceToolTotal,
			})
		})
		return nil
	}); err != nil {
		return fmt.Errorf("register activity hook: %w", err)
	}
	m.goBackground(func() {
		if err := autoEngine.DrainActivityInbox(context.Background()); err != nil {
			m.logger.Error("drain durable activity inbox failed", "workspace", meta.ID, "error", err)
		}
	})

	// Restart-safe: continue any flow runs interrupted by a previous shutdown.
	rt.ResumeRunningFlows(context.Background())
	// Timeout sweeper: fail await-input runs that out-wait their node's TimeoutSec.
	rt.StartWaitingFlowSweeper(context.Background())
	// Durable Ask timeout sweeper: close ask/permission cards nobody answered in
	// time (no-op until a TimeoutSec is stamped — 0 = unlimited by default).
	rt.StartWaitingAskSweeper(context.Background())
	// Coordinator stall sweeper: the long-horizon backstop for a coordinator frozen
	// after narrating a spawn it never issued (no-op while the guard/window is off).
	rt.StartCoordinatorStallSweeper(context.Background())
	// Rota F3: weekly, idle-triggered curator pass (archive-only, pin-aware).
	rt.StartCuratorSweeper(context.Background())

	ws := &Workspace{Meta: meta, DB: database, Runtime: rt, Scheduler: sched, InsightCron: insightCron, Secrets: vault, DataDir: dir}
	ws.loadSettings() // apply persisted per-workspace overrides (e.g. autonomy pause)
	// Self-heal a defaultAgentId written before system agents were barred from it.
	ws.sanitizeDefaultAgent(m.logger)
	ws.syncConfigFiles() // seed config/ tree + adopt instructions.md (file is authoritative)

	// One board dispatcher fans out to automation and the card-owned worktree
	// lifecycle. Both consumers run detached; SetBoardHook remains a single hook.
	worktreeSettings := ws.Settings()
	repoRoot := worktreeSettings.DefaultWorkingDir
	if repoRoot == "" {
		repoRoot = ws.SandboxRoot()
	}
	worktreeRoot := worktreeSettings.WorktreeRootDir
	if worktreeRoot == "" {
		worktreeRoot = filepath.Join(filepath.Dir(repoRoot), ".tionharness-worktrees", meta.ID)
	}
	cardWorktrees := &worktree.Lifecycle{
		Store: database,
		Git: worktree.Git{
			RepoRoot: repoRoot,
			Runner:   worktree.ExecRunner{},
		},
		BaseRef:      worktreeSettings.WorktreeBaseRef,
		WorktreeRoot: worktreeRoot,
	}
	database.SetBoardHook(func(ev db.BoardChangeEvent) {
		m.goBackground(func() { autoEngine.OnBoardChange(context.Background(), ev) })
		m.goBackground(func() {
			if err := cardWorktrees.Handle(context.Background(), ev); err != nil {
				m.logger.Error("card worktree lifecycle failed", "workspace", meta.ID, "task", ev.TaskID, "error", err)
			}
		})
	})

	m.mu.Lock()
	m.workspaces[meta.ID] = ws
	m.order = append(m.order, meta.ID)
	// It opened, so it is no longer degraded (a reopen after a repair, or a
	// restore from archive).
	m.degraded = removeDegraded(m.degraded, meta.ID)
	m.mu.Unlock()

	m.logger.Info("workspace opened", "id", meta.ID, "name", meta.Name)
	return nil
}

// List returns LIVE workspace metadata in creation order. Degraded workspaces are
// deliberately absent: every caller of this pairs a Meta with Get(meta.ID) or a
// runtime handle, which a degraded entry does not have. Use ListWithDegraded for
// the registry as the user should see it.
func (m *Manager) List() []Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Meta, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.workspaces[id].Meta)
	}
	return out
}

// ListWithDegraded returns the whole registry in creation order — live workspaces
// and the ones that failed to open, each flagged. Without this the user has no
// way to see a broken workspace at all (it is still in workspaces.json but has no
// live handle), so it could neither be repaired nor deleted from the UI.
func (m *Manager) ListWithDegraded() []ListEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ListEntry, 0, len(m.order)+len(m.degraded))
	for _, id := range m.order {
		out = append(out, ListEntry{Meta: m.workspaces[id].Meta})
	}
	for _, d := range m.degraded {
		out = append(out, ListEntry{Meta: d.Meta, Degraded: true, Reason: d.Reason})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// BackupTarget describes one workspace's on-disk location for the backup
// subsystem: its id, label and the absolute data directory that holds all of
// the workspace's content (store/, config/, workspace/, ws-settings.json).
type BackupTarget struct {
	ID   string
	Name string
	Dir  string
}

// BackupTargets returns the data directory of every workspace so the backup
// manager can snapshot them. The path mirrors open(): a user-chosen Path, or
// the default per-workspace folder under the manager root.
func (m *Manager) BackupTargets() []BackupTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]BackupTarget, 0, len(m.order))
	for _, id := range m.order {
		ws := m.workspaces[id]
		dir := ws.DataDir
		if dir == "" {
			dir = ws.Meta.Path
			if dir == "" {
				dir = filepath.Join(m.rootDir, "workspaces", ws.Meta.ID)
			}
		}
		out = append(out, BackupTarget{ID: ws.Meta.ID, Name: ws.Meta.Name, Dir: dir})
	}
	return out
}

// Get returns a workspace by id.
func (m *Manager) Get(id string) (*Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ws, ok := m.workspaces[id]
	if !ok {
		return nil, errors.New("workspace not found")
	}
	return ws, nil
}

// Default returns the first (default) workspace.
func (m *Manager) Default() *Workspace {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.order) == 0 {
		return nil
	}
	return m.workspaces[m.order[0]]
}

// Create makes a new isolated workspace. parentPath, when non-empty, is a
// user-chosen directory under which this workspace's own data folder is created
// (so deleting the workspace never removes unrelated sibling content); empty
// uses the default location under the manager root. createdBy stamps provenance:
// pass an agent id when an agent creates the workspace (so it may delete it
// later), or "" for a user-created workspace.
func (m *Manager) Create(name, parentPath, createdBy string) (*Workspace, error) {
	if name == "" {
		name = "Yeni Workspace"
	}
	meta := Meta{ID: m.nextWorkspaceID(), Name: name, CreatedAt: time.Now().Unix(), CreatedBy: createdBy}
	if parentPath != "" {
		meta.Path = filepath.Join(parentPath, "tionharness-"+meta.ID)
	}
	if err := m.open(meta); err != nil {
		return nil, err
	}
	if err := m.persist(m.registryMetas()); err != nil {
		return nil, err
	}
	return m.Get(meta.ID)
}

// ErrNotAWorkspace is returned by Attach when the chosen folder does not look
// like a TionHarness workspace data directory (it lacks a store/ subfolder).
var ErrNotAWorkspace = errors.New("seçilen klasör geçerli bir workspace değil (içinde store/ klasörü yok)")

// Attach registers an EXISTING on-disk workspace data directory as a workspace
// without moving or recreating its content — used to adopt a folder from a prior
// install or another machine (copied verbatim). The folder must already contain a
// store/ subdirectory (the file-based DB). A fresh registry id is issued and its
// Path points directly at the chosen folder, so open() reuses the existing
// store/config/workspace content in place. Returns ErrNotAWorkspace for an
// invalid folder, and an error if the same folder is already attached.
func (m *Manager) Attach(path string) (*Workspace, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("klasör yolu boş")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !isWorkspaceDir(abs) {
		return nil, ErrNotAWorkspace
	}

	// Reject a folder that is already an attached workspace (same data dir), so the
	// same content is never registered under two ids. Degraded workspaces count:
	// they are registered too, just not open, so attaching one's folder again would
	// give the same directory two registry entries.
	m.mu.RLock()
	for _, ws := range m.workspaces {
		if sameDir(ws.DataDir, abs) {
			name := ws.Meta.Name
			m.mu.RUnlock()
			return nil, fmt.Errorf("bu klasör zaten '%s' workspace'i olarak ekli", name)
		}
	}
	for _, d := range m.degraded {
		if sameDir(m.workspaceDir(d.Meta), abs) {
			name := d.Name
			m.mu.RUnlock()
			return nil, fmt.Errorf("bu klasör zaten '%s' workspace'i olarak ekli", name)
		}
	}
	// A directory whose files are being erased right now must not be adopted: the
	// registry record is already gone, so the loops above cannot see it, and the
	// in-flight os.RemoveAll would delete the newly attached workspace's data.
	for _, dir := range m.pendingRemoval {
		if sameDir(dir, abs) {
			m.mu.RUnlock()
			return nil, errors.New("bu klasör şu anda siliniyor, yeniden eklenemez")
		}
	}
	m.mu.RUnlock()

	meta := Meta{ID: m.nextWorkspaceID(), Name: workspaceNameFromDir(abs), CreatedAt: time.Now().Unix(), Path: abs}
	if err := m.open(meta); err != nil {
		return nil, err
	}
	if err := m.persist(m.registryMetas()); err != nil {
		return nil, err
	}
	return m.Get(meta.ID)
}

// workspaceDir returns the data directory a registry entry lives in: the
// user-chosen Path when it has one, otherwise the default per-workspace location
// under rootDir. Same rule as open(), so a degraded entry (which has no live
// DataDir) resolves to the directory it would have been opened from.
func (m *Manager) workspaceDir(meta Meta) string {
	if meta.Path != "" {
		return meta.Path
	}
	return filepath.Join(m.rootDir, "workspaces", meta.ID)
}

// beginRemovalLocked marks dir as being erased. The caller must hold m.mu for
// writing, in the same hold that drops the directory's registry entry.
func (m *Manager) beginRemovalLocked(dir string) {
	m.pendingRemoval = append(m.pendingRemoval, dir)
}

// endRemoval clears the mark set by beginRemovalLocked, whether the removal
// succeeded or not: once the delete has returned, nothing is writing to the
// directory any more.
func (m *Manager) endRemoval(dir string) {
	m.mu.Lock()
	for i, p := range m.pendingRemoval {
		if p == dir {
			m.pendingRemoval = append(m.pendingRemoval[:i], m.pendingRemoval[i+1:]...)
			break
		}
	}
	m.mu.Unlock()
}

// isWorkspaceDir reports whether dir is a plausible TionHarness workspace data
// directory: it exists and contains a store/ subdirectory (the file-based DB
// every workspace owns). This is the single validity signal the onboarding
// "select existing workspace" flow relies on.
func isWorkspaceDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "store"))
	return err == nil && info.IsDir()
}

// workspaceNameFromDir derives a display name from a workspace folder path,
// stripping the "tionharness-" prefix Create() adds so an attached folder reads
// back with a sensible label. The original name is not stored inside the folder
// (it lived in the source install's registry), so the user may rename afterward.
func workspaceNameFromDir(dir string) string {
	base := strings.TrimPrefix(filepath.Base(dir), "tionharness-")
	if strings.TrimSpace(base) == "" {
		return "Eklenen Workspace"
	}
	return base
}

// sameDir compares two directory paths for equality after cleaning, case-
// insensitively on Windows (whose filesystem paths are case-insensitive).
func sameDir(a, b string) bool {
	ca, cb := filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(ca, cb)
	}
	return ca == cb
}

// Delete removes a workspace and all its data. Deleting the last workspace IS
// allowed: the manager then holds zero workspaces and the web UI falls back to
// the first-run onboarding screen (no default workspace is re-seeded).
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	ws, ok := m.workspaces[id]
	if !ok {
		m.mu.Unlock()
		return m.deleteDegraded(id)
	}
	delete(m.workspaces, id)
	m.order = removeString(m.order, id)
	m.beginRemovalLocked(ws.DataDir)
	m.mu.Unlock()
	defer m.endRemoval(ws.DataDir)

	ws.Scheduler.Stop()
	if ws.InsightCron != nil {
		ws.InsightCron.Stop()
	}
	ws.Runtime.CloseMCP()
	_ = ws.DB.Close()
	if err := os.RemoveAll(ws.DataDir); err != nil {
		m.logger.Warn("failed to remove workspace dir", "id", id, "error", err)
	}
	// No lock is held here on purpose: stopping the scheduler, closing the DB and
	// removing the directory must not run under m.mu. registryMetas takes the read
	// lock only for the snapshot.
	return m.persist(m.registryMetas())
}

// deleteDegraded removes a registered-but-unopenable workspace. Without it a
// degraded entry is undeletable: it is absent from m.workspaces, so Delete used to
// answer "workspace not found" and the broken record stayed in workspaces.json
// forever. The registry entry is dropped and persisted FIRST — that is the part
// the user asked for and the part that must survive — then the data directory is
// removed with the same semantics as the live path (delete, not archive). Unlike
// the live path the removal error is returned rather than logged: a degraded
// workspace is degraded precisely because its directory is suspect, so "deleted"
// must not be reported when its files are still there.
//
// A failed persist() must leave NOTHING changed: the entry is still in
// workspaces.json, so it is put back into m.degraded at its original position.
// Dropping it from memory only would let the next successful persist (a Create,
// say) erase a registry entry the user never managed to delete.
//
// The removal, the write and the rollback all happen in ONE write-lock hold, so
// the half-deleted state is never observable: a concurrent ListWithDegraded
// either sees the workspace registered or sees it gone, never "missing but about
// to come back". That is only possible because persist takes its snapshot as an
// argument instead of locking internally. The directory removal stays OUTSIDE the
// hold — it is slow, destructive IO on a directory nothing points at any more once
// persist has succeeded, and holding m.mu across it would stall every reader.
func (m *Manager) deleteDegraded(id string) error {
	m.mu.Lock()
	var entry DegradedWorkspace
	idx := -1
	for i, d := range m.degraded {
		if d.ID == id {
			entry, idx = d, i
			break
		}
	}
	if idx < 0 {
		m.mu.Unlock()
		return errors.New("workspace not found")
	}
	m.degraded = removeDegraded(m.degraded, id)
	if err := m.persist(m.registryMetasLocked()); err != nil {
		m.degraded = insertDegraded(m.degraded, idx, entry)
		m.mu.Unlock()
		return err
	}
	// Claim the directory before the lock goes: from here on no registry entry
	// points at it, so only pendingRemoval can stop an Attach from adopting a
	// folder this call is about to erase.
	dir := m.workspaceDir(entry.Meta)
	m.beginRemovalLocked(dir)
	m.mu.Unlock()
	defer m.endRemoval(dir)

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove degraded workspace dir %s: %w", dir, err)
	}
	return nil
}

// RestoreFromArchive replaces a workspace's on-disk content with the contents of
// a backup archive and reopens it live. The flow: detach the workspace (stop
// scheduler, close MCP + DB, remove from the registry) → extract the archive to
// a staging dir → atomically swap the restored content (store/, config/,
// workspace/, ws-settings.json) into place → reopen from disk. extract is the
// archive-format-specific unzip (injected so this package stays decoupled from
// the backup format). On a failed extraction the original workspace is reopened
// unchanged. The workspace id/identity is preserved.
//
// Caller note: restoring the workspace currently open in a UI desyncs that
// view's in-memory state; the client should reload after a restore.
func (m *Manager) RestoreFromArchive(id, archivePath string, extract func(src, dst string) error) error {
	m.mu.Lock()
	ws, ok := m.workspaces[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("workspace not found")
	}
	meta := ws.Meta
	dataDir := ws.DataDir
	// Detach from the registry while we rewrite its files.
	delete(m.workspaces, id)
	m.order = removeString(m.order, id)
	// Same window as Delete/deleteDegraded: from here until the workspace is
	// reopened no registry entry points at dataDir, while the swap below erases
	// its live content (os.RemoveAll per top-level entry) outside the lock. Only
	// pendingRemoval can stop an Attach from adopting a folder whose files are
	// being torn out from under it — and from giving that directory a second
	// registry entry once open() re-registers the original id.
	m.beginRemovalLocked(dataDir)
	m.mu.Unlock()
	defer m.endRemoval(dataDir)

	// Release all live handles so the files can be replaced (Windows locks open
	// files). The scheduler/runtime/DB are recreated by open() at the end.
	ws.Scheduler.Stop()
	if ws.InsightCron != nil {
		ws.InsightCron.Stop()
	}
	ws.Runtime.CloseMCP()
	_ = ws.DB.Close()

	// Stage the extraction in a sibling temp dir so a mid-extraction failure
	// never leaves the workspace half-wiped.
	staging := dataDir + ".restore-stage"
	_ = os.RemoveAll(staging)
	if err := extract(archivePath, staging); err != nil {
		_ = os.RemoveAll(staging)
		m.reopenOrLog(meta) // roll back: reopen the untouched workspace
		return fmt.Errorf("extract archive: %w", err)
	}

	// Swap each restored top-level entry into place: remove the live copy then
	// move the staged copy over it (same volume → fast rename).
	swapErr := func() error {
		entries, err := os.ReadDir(staging)
		if err != nil {
			return err
		}
		for _, e := range entries {
			dst := filepath.Join(dataDir, e.Name())
			src := filepath.Join(staging, e.Name())
			if err := os.RemoveAll(dst); err != nil {
				return err
			}
			if err := os.Rename(src, dst); err != nil {
				return err
			}
		}
		return nil
	}()
	_ = os.RemoveAll(staging)
	if swapErr != nil {
		// Content may be partially swapped; reopen whatever is on disk so the
		// workspace is at least live again, and surface the error.
		m.reopenOrLog(meta)
		return fmt.Errorf("swap restored content: %w", swapErr)
	}

	if err := m.open(meta); err != nil {
		// The content was swapped in but the workspace will not open: keep its
		// registry entry so a later fix can still reach it.
		m.markDegraded(meta, err)
		return fmt.Errorf("reopen workspace after restore: %w", err)
	}
	m.logger.Info("workspace restored from archive", "id", id, "archive", filepath.Base(archivePath))
	return nil
}

// reopenOrLog re-registers a previously detached workspace, logging on failure
// (used on restore rollback paths where the caller already has an error to
// return — losing the workspace from the registry would be worse than a log).
func (m *Manager) reopenOrLog(meta Meta) {
	if err := m.open(meta); err != nil {
		m.logger.Error("failed to reopen workspace after restore rollback", "id", meta.ID, "error", err)
		m.markDegraded(meta, err)
	}
}

// markDegraded records a registered-but-unopenable workspace so persist() keeps
// its entry in workspaces.json and ListWithDegraded can surface it. cause is the
// open failure and must not be nil — it is the only explanation the user gets.
// Re-marking an already degraded workspace refreshes the reason.
func (m *Manager) markDegraded(meta Meta, cause error) {
	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, d := range m.degraded {
		if d.ID == meta.ID {
			m.degraded[i] = DegradedWorkspace{Meta: meta, Reason: reason}
			return
		}
	}
	m.degraded = append(m.degraded, DegradedWorkspace{Meta: meta, Reason: reason})
}

// insertDegraded puts an entry back at index idx, preserving the rest of the
// order (used to roll back a removal whose persist failed).
func insertDegraded(list []DegradedWorkspace, idx int, entry DegradedWorkspace) []DegradedWorkspace {
	if idx > len(list) {
		idx = len(list)
	}
	list = append(list, DegradedWorkspace{})
	copy(list[idx+1:], list[idx:])
	list[idx] = entry
	return list
}

func removeDegraded(list []DegradedWorkspace, id string) []DegradedWorkspace {
	out := list[:0]
	for _, d := range list {
		if d.ID != id {
			out = append(out, d)
		}
	}
	return out
}

// goBackground runs fn on a detached goroutine Close() will wait for. Work
// dispatched after Close has started draining is DROPPED rather than started:
// the process (or the test) is on its way out, and a late goroutine is exactly
// the one that would touch the store directory after it is gone.
func (m *Manager) goBackground(fn func()) {
	m.bgMu.Lock()
	if m.bgClosed {
		m.bgMu.Unlock()
		return
	}
	m.bg.Add(1)
	m.bgMu.Unlock()
	go func() {
		defer m.bg.Done()
		fn()
	}()
}

// Close stops every workspace's runtime and closes its database.
//
// The hook-dispatched background work is drained FIRST, while the stores are
// still open: a drain that ran against an already-closed DB would only log a
// failure for work it could no longer finish.
func (m *Manager) Close() {
	m.bgMu.Lock()
	m.bgClosed = true
	m.bgMu.Unlock()
	m.bg.Wait()

	// Snapshot under the lock, tear down OUTSIDE it — like Delete and
	// RestoreFromArchive already do. CloseMCP now waits for the in-flight background
	// turns, and such a turn may reach back into the manager (settings bridge, task
	// lookups): holding the write lock across that wait would deadlock the shutdown
	// against the very turns it is waiting for.
	m.mu.Lock()
	open := make([]*Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		open = append(open, ws)
	}
	m.mu.Unlock()

	for _, ws := range open {
		ws.Scheduler.Stop()
		if ws.InsightCron != nil {
			ws.InsightCron.Stop()
		}
		ws.Runtime.CloseMCP()
		_ = ws.DB.Close()
	}
}

// ---- persistence ----

func (m *Manager) metaPath() string {
	return filepath.Join(m.rootDir, "workspaces.json")
}

// loadMetas reads the workspace registry. ONLY a missing file is an empty
// registry; an unreadable or unparseable one is an error that must abort the
// boot. Treating a corrupt workspaces.json as "no workspaces" would let the
// first persist() overwrite it with an empty list and erase every workspace the
// user has.
func (m *Manager) loadMetas() ([]Meta, error) {
	data, err := os.ReadFile(m.metaPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read workspace registry %s: %w", m.metaPath(), err)
	}
	var metas []Meta
	if err := json.Unmarshal(data, &metas); err != nil {
		return nil, fmt.Errorf("workspace registry %s is corrupt: %w", m.metaPath(), err)
	}
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].CreatedAt < metas[j].CreatedAt })
	return metas, nil
}

// registryMetas is everything workspaces.json must contain: the live workspaces
// plus the degraded ones, in creation order. For call sites that hold no lock of
// their own; anything already holding m.mu must use registryMetasLocked.
func (m *Manager) registryMetas() []Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.registryMetasLocked()
}

// registryMetasLocked is registryMetas without taking the lock. m.mu (read or
// write) must already be held: sync.RWMutex is not reentrant, so a caller that
// wants the snapshot and the mutation it describes in ONE hold — deleteDegraded's
// remove/persist/rollback — has no other way to get it.
func (m *Manager) registryMetasLocked() []Meta {
	out := make([]Meta, 0, len(m.order)+len(m.degraded))
	for _, id := range m.order {
		out = append(out, m.workspaces[id].Meta)
	}
	for _, d := range m.degraded {
		out = append(out, d.Meta)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out
}

// persist writes the given registry snapshot atomically (temp file + rename).
// This file is the only pointer to every workspace's data directory: a
// half-written workspaces.json left by a crash mid-write is unrecoverable, so it
// is never written in place.
//
// It takes the snapshot as an argument and acquires NO lock, so a caller may hold
// m.mu across the write. That is what lets deleteDegraded keep its removal, the
// write and the rollback in a single hold; a persist that locked internally could
// never be called from one.
func (m *Manager) persist(metas []Meta) error {
	data, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		return err
	}
	path := m.metaPath()
	tmp := path + ".tmp"
	// Every failure path removes the temp file: leaving one behind next to the
	// registry is silent garbage that the next boot has no owner for. The removal
	// itself is best-effort — the write/rename error is the one worth reporting.
	if err := writeSynced(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// writeSynced writes data to path and fsyncs it before returning. os.WriteFile
// would be shorter but leaves the bytes in the page cache: renaming an unflushed
// temp file over the registry survives a power cut as a ZERO-BYTE workspaces.json,
// which is the exact loss the temp-file+rename dance exists to prevent.
func writeSynced(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func removeString(s []string, v string) []string {
	out := s[:0]
	for _, x := range s {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

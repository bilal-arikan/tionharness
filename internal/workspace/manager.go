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

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/logbuf"
	"github.com/bilal-arikan/tionswarm/internal/providers"
	"github.com/bilal-arikan/tionswarm/internal/secrets"
	"github.com/bilal-arikan/tionswarm/internal/tools"
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

	// wsCounter is the monotonic sequence behind human-readable workspace ids
	// ("WS1", "WS2"), persisted to ws-counter.json so a number is never reused
	// across deletions or restarts. Guarded by mu.
	wsCounter int64

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

	// wakeTurnFactory builds the history-aware self-wake turn runner for a runtime
	// (so schedule_wake continues with the full conversation, not just the wake
	// prompt). Wired in after the api server exists; applied to existing +
	// later-opened runtimes. nil until wired.
	wakeTurnFactory func(*agent.Runtime) agent.WakeTurnFunc
}

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

	metas, err := m.loadMetas()
	if err != nil {
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
			logger.Warn("failed to open workspace", "id", meta.ID, "error", err)
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

	// Provision this workspace's per-workspace claude-cli config home
	// (<workspace>/claude-home): seed it from the global home on first open and
	// migrate any legacy <workspace>/skills into it. Must run BEFORE NewRuntime so
	// the skill store scans the migrated (populated) tier. Idempotent.
	agent.EnsureWorkspaceClaudeHome(dir)

	storeDir := filepath.Join(dir, "store")
	database, err := db.Open(storeDir)
	if err != nil {
		return err
	}

	// Seed the built-in default flows into this workspace's store. Idempotent and
	// deletion-aware (a ledger keeps a user-removed default from coming back).
	// Running it here backfills every existing workspace on the next startup.
	if err := agent.EnsureDefaultFlows(context.Background(), database, storeDir); err != nil {
		m.logger.Warn("seed default flows failed", "workspace", meta.ID, "error", err)
	}
	// Upgrade existing flows to the required start-node model (adds a start node
	// where missing). Idempotent; backfills every workspace on the next startup.
	if err := agent.MigrateFlowsStartEnd(context.Background(), database); err != nil {
		m.logger.Warn("migrate flows to start-node model failed", "workspace", meta.ID, "error", err)
	}

	// Per-workspace secret vault (AES-GCM encrypted), shared by the secret_* tools.
	vault, err := secrets.Open(storeDir, m.cipher)
	if err != nil {
		return err
	}

	rt := agent.NewRuntime(database, m.registry, m.tun, filepath.Join(dir, "workspace"), vault, m.bus, meta.ID, meta.Name, m.logs, m.logger)

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
	// Board-triggered automations: a kanban card change (create/move/update/delete)
	// fires the engine on a detached goroutine so the mutation is never blocked.
	database.SetBoardHook(func(ev db.BoardChangeEvent) {
		go autoEngine.OnBoardChange(context.Background(), ev)
	})

	// Restart-safe: continue any flow runs interrupted by a previous shutdown.
	rt.ResumeRunningFlows(context.Background())
	// Timeout sweeper: fail await-input runs that out-wait their node's TimeoutSec.
	rt.StartWaitingFlowSweeper(context.Background())
	// Durable Ask timeout sweeper: close ask/permission cards nobody answered in
	// time (no-op until a TimeoutSec is stamped — 0 = unlimited by default).
	rt.StartWaitingAskSweeper(context.Background())

	ws := &Workspace{Meta: meta, DB: database, Runtime: rt, Scheduler: sched, InsightCron: insightCron, Secrets: vault, DataDir: dir}
	ws.loadSettings()    // apply persisted per-workspace overrides (e.g. autonomy pause)
	ws.syncConfigFiles() // seed config/ tree + adopt instructions.md (file is authoritative)

	m.mu.Lock()
	m.workspaces[meta.ID] = ws
	m.order = append(m.order, meta.ID)
	m.mu.Unlock()

	m.logger.Info("workspace opened", "id", meta.ID, "name", meta.Name)
	return nil
}

// List returns workspace metadata in creation order.
func (m *Manager) List() []Meta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Meta, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.workspaces[id].Meta)
	}
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
		meta.Path = filepath.Join(parentPath, "tionswarm-"+meta.ID)
	}
	if err := m.open(meta); err != nil {
		return nil, err
	}
	if err := m.persist(); err != nil {
		return nil, err
	}
	return m.Get(meta.ID)
}

// ErrNotAWorkspace is returned by Attach when the chosen folder does not look
// like a TionSwarm workspace data directory (it lacks a store/ subfolder).
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
	// same content is never registered under two ids.
	m.mu.RLock()
	for _, ws := range m.workspaces {
		if sameDir(ws.DataDir, abs) {
			name := ws.Meta.Name
			m.mu.RUnlock()
			return nil, fmt.Errorf("bu klasör zaten '%s' workspace'i olarak ekli", name)
		}
	}
	m.mu.RUnlock()

	meta := Meta{ID: m.nextWorkspaceID(), Name: workspaceNameFromDir(abs), CreatedAt: time.Now().Unix(), Path: abs}
	if err := m.open(meta); err != nil {
		return nil, err
	}
	if err := m.persist(); err != nil {
		return nil, err
	}
	return m.Get(meta.ID)
}

// isWorkspaceDir reports whether dir is a plausible TionSwarm workspace data
// directory: it exists and contains a store/ subdirectory (the file-based DB
// every workspace owns). This is the single validity signal the onboarding
// "select existing workspace" flow relies on.
func isWorkspaceDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "store"))
	return err == nil && info.IsDir()
}

// workspaceNameFromDir derives a display name from a workspace folder path,
// stripping the "tionswarm-" prefix Create() adds so an attached folder reads
// back with a sensible label. The original name is not stored inside the folder
// (it lived in the source install's registry), so the user may rename afterward.
func workspaceNameFromDir(dir string) string {
	base := strings.TrimPrefix(filepath.Base(dir), "tionswarm-")
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
		return errors.New("workspace not found")
	}
	delete(m.workspaces, id)
	m.order = removeString(m.order, id)
	m.mu.Unlock()

	ws.Scheduler.Stop()
	if ws.InsightCron != nil {
		ws.InsightCron.Stop()
	}
	ws.Runtime.CloseMCP()
	_ = ws.DB.Close()
	if err := os.RemoveAll(ws.DataDir); err != nil {
		m.logger.Warn("failed to remove workspace dir", "id", id, "error", err)
	}
	return m.persist()
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
	m.mu.Unlock()

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
	}
}

// Close stops every workspace's runtime and closes its database.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ws := range m.workspaces {
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

func (m *Manager) loadMetas() ([]Meta, error) {
	data, err := os.ReadFile(m.metaPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var metas []Meta
	if err := json.Unmarshal(data, &metas); err != nil {
		return nil, err
	}
	sort.SliceStable(metas, func(i, j int) bool { return metas[i].CreatedAt < metas[j].CreatedAt })
	return metas, nil
}

func (m *Manager) persist() error {
	metas := m.List()
	data, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.metaPath(), data, 0o644)
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

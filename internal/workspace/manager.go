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
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/bilal/swarmgo/internal/agent"
	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/events"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/secrets"
)

// Meta is the persisted descriptor of a workspace (no live handles). Path, when
// set, is the workspace's data directory chosen by the user; empty means the
// default location under the manager root.
type Meta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int64  `json:"createdAt"`
	Path      string `json:"path,omitempty"`
}

// Workspace bundles a workspace's live database, runtime and scheduler.
type Workspace struct {
	Meta
	DB        *db.DB
	Runtime   *agent.Runtime
	Scheduler *agent.Scheduler
	Secrets   *secrets.Vault
	DataDir   string

	settings settingsHolder // per-workspace overrides (ws-settings.json)
}

// Manager owns all workspaces and persists their registry.
type Manager struct {
	rootDir  string
	registry *providers.Registry
	tun      *agent.Tunables
	cipher   secrets.Cipher
	bus      *events.Bus
	logger   *slog.Logger

	mu         sync.RWMutex
	workspaces map[string]*Workspace
	order      []string // creation order (first = default)
}

// NewManager loads the registry from disk, opens every workspace, and ensures
// at least one default workspace exists. tun is the shared process-wide
// tunables handed to every workspace runtime; bus is the process-wide event bus
// each runtime publishes autonomous notifications to.
func NewManager(rootDir string, registry *providers.Registry, tun *agent.Tunables, cipher secrets.Cipher, bus *events.Bus, logger *slog.Logger) (*Manager, error) {
	m := &Manager{
		rootDir:    rootDir,
		registry:   registry,
		tun:        tun,
		cipher:     cipher,
		bus:        bus,
		logger:     logger,
		workspaces: make(map[string]*Workspace),
	}

	metas, err := m.loadMetas()
	if err != nil {
		return nil, err
	}

	for _, meta := range metas {
		if err := m.open(meta); err != nil {
			logger.Warn("failed to open workspace", "id", meta.ID, "error", err)
			continue
		}
	}

	// Ensure a default workspace exists.
	if len(m.order) == 0 {
		if _, err := m.Create("Varsayılan", ""); err != nil {
			return nil, fmt.Errorf("create default workspace: %w", err)
		}
	}

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

	storeDir := filepath.Join(dir, "store")
	database, err := db.Open(storeDir)
	if err != nil {
		return err
	}

	// Per-workspace secret vault (AES-GCM encrypted), shared by the secret_* tools.
	vault, err := secrets.Open(storeDir, m.cipher)
	if err != nil {
		return err
	}

	rt := agent.NewRuntime(database, m.registry, m.tun, filepath.Join(dir, "workspace"), vault, m.bus, meta.ID, meta.Name, m.logger)
	if err := rt.StartConfigured(context.Background()); err != nil {
		m.logger.Warn("start configured agents failed", "workspace", meta.ID, "error", err)
	}

	sched := agent.NewScheduler(database, rt, m.logger)
	if err := sched.Start(context.Background()); err != nil {
		m.logger.Warn("start scheduler failed", "workspace", meta.ID, "error", err)
	}

	// Restart-safe: continue any flow runs interrupted by a previous shutdown.
	rt.ResumeRunningFlows(context.Background())

	ws := &Workspace{Meta: meta, DB: database, Runtime: rt, Scheduler: sched, Secrets: vault, DataDir: dir}
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
// uses the default location under the manager root.
func (m *Manager) Create(name, parentPath string) (*Workspace, error) {
	if name == "" {
		name = "Yeni Workspace"
	}
	meta := Meta{ID: uuid.NewString(), Name: name, CreatedAt: time.Now().Unix()}
	if parentPath != "" {
		meta.Path = filepath.Join(parentPath, "swarmgo-"+meta.ID)
	}
	if err := m.open(meta); err != nil {
		return nil, err
	}
	if err := m.persist(); err != nil {
		return nil, err
	}
	return m.Get(meta.ID)
}

// Delete removes a workspace and all its data. The last workspace cannot be
// deleted (there must always be at least one).
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	if len(m.order) <= 1 {
		m.mu.Unlock()
		return errors.New("cannot delete the last workspace")
	}
	ws, ok := m.workspaces[id]
	if !ok {
		m.mu.Unlock()
		return errors.New("workspace not found")
	}
	delete(m.workspaces, id)
	m.order = removeString(m.order, id)
	m.mu.Unlock()

	ws.Scheduler.Stop()
	ws.Runtime.StopAll()
	_ = ws.DB.Close()
	if err := os.RemoveAll(ws.DataDir); err != nil {
		m.logger.Warn("failed to remove workspace dir", "id", id, "error", err)
	}
	return m.persist()
}

// Close stops every workspace's runtime and closes its database.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ws := range m.workspaces {
		ws.Scheduler.Stop()
		ws.Runtime.StopAll()
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

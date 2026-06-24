// Package backup provides periodic, retention-bounded snapshots of every
// workspace's data directory. Each run zips a workspace's folder into a
// timestamped archive under a backups root; old archives beyond the retention
// count are pruned. The manager is process-wide (one per app), reads its live
// configuration from the settings store, and is reconfigured whenever settings
// change. Backups never block the main flow: a run failure is logged, not fatal.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Target is one workspace to back up: a stable id, a human label and the
// absolute path of its data directory (the folder that holds store/, config/,
// workspace/, ...).
type Target struct {
	ID   string
	Name string
	Dir  string
}

// Lister returns the current set of workspaces to back up. It is called fresh
// before every run so workspaces created/deleted after start are picked up.
type Lister func() []Target

// Config is the live, user-editable backup configuration mirrored from settings.
type Config struct {
	Enabled       bool   // master switch
	IntervalHours int    // hours between automatic runs (min 1)
	Retain        int    // newest archives kept per workspace (min 1)
	Dir           string // backups root; "" → <dataDir>/backups
}

// Manager runs the periodic backup loop and serves on-demand runs.
type Manager struct {
	dataDir string
	list    Lister
	logger  *slog.Logger

	mu      sync.Mutex
	cfg     Config
	cancel  context.CancelFunc // stops the current loop; nil when not running
	lastRun time.Time
	lastErr string
	running bool // a backup pass is currently executing (guards manual+timer overlap)

	// runMu serialises whole backup passes so a manual run during a scheduled run
	// (or vice versa) never writes the same archive path twice. Held for the full
	// duration of RunOnce; distinct from mu which only guards the small fields.
	runMu sync.Mutex
}

// New builds a backup manager. dataDir is the app data directory used to resolve
// the default backups root; list enumerates the workspaces to snapshot.
func New(dataDir string, list Lister, logger *slog.Logger) *Manager {
	return &Manager{dataDir: dataDir, list: list, logger: logger}
}

// Configure applies a new configuration and (re)starts or stops the loop to
// match. It is safe to call repeatedly (boot + every settings change).
func (m *Manager) Configure(cfg Config) {
	if cfg.IntervalHours < 1 {
		cfg.IntervalHours = 1
	}
	if cfg.Retain < 1 {
		cfg.Retain = 1
	}

	m.mu.Lock()
	prev := m.cfg
	m.cfg = cfg
	// Stop any existing loop before deciding whether to restart.
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if !cfg.Enabled {
		m.mu.Unlock()
		if prev.Enabled {
			m.logger.Info("backup disabled")
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	interval := time.Duration(cfg.IntervalHours) * time.Hour
	m.mu.Unlock()

	m.logger.Info("backup enabled", "interval_hours", cfg.IntervalHours, "retain", cfg.Retain, "dir", m.resolveDir(cfg))
	go m.loop(ctx, interval)
}

// loop fires a backup pass every interval until the context is cancelled. The
// first pass runs one interval after (re)configuration, not immediately, so a
// restart storm never triggers a burst of snapshots.
func (m *Manager) loop(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := m.RunOnce(ctx); err != nil {
				m.logger.Warn("scheduled backup failed", "error", err)
			}
		}
	}
}

// Stop halts the periodic loop (graceful shutdown). In-flight passes finish.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// Result summarises a backup pass for the API/manual trigger.
type Result struct {
	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"`
	Dir      string            `json:"dir"`
	Archives []ArchiveInfo     `json:"archives"`
	Failures map[string]string `json:"failures,omitempty"` // workspace id → error
}

// ArchiveInfo describes one written archive.
type ArchiveInfo struct {
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	Path          string `json:"path"`
	Bytes         int64  `json:"bytes"`
}

// RunOnce performs a single backup pass over every current workspace, writing
// one archive each and pruning old ones. It is safe to call manually and from
// the loop; overlapping calls are serialised (the second waits, it is not
// dropped) so a manual run during a scheduled run never corrupts an archive.
func (m *Manager) RunOnce(ctx context.Context) (Result, error) {
	m.runMu.Lock()
	defer m.runMu.Unlock()

	m.mu.Lock()
	cfg := m.cfg
	root := m.resolveDir(cfg)
	m.running = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	res := Result{Started: time.Now(), Dir: root, Failures: map[string]string{}}

	if err := os.MkdirAll(root, 0o755); err != nil {
		m.recordRun(time.Now(), err.Error())
		return res, fmt.Errorf("create backup dir: %w", err)
	}

	stamp := res.Started.Format("20060102-150405")
	for _, tgt := range m.list() {
		if ctx.Err() != nil {
			break
		}
		if tgt.Dir == "" {
			continue
		}
		wsRoot := filepath.Join(root, tgt.ID)
		if err := os.MkdirAll(wsRoot, 0o755); err != nil {
			res.Failures[tgt.ID] = err.Error()
			continue
		}
		archive := filepath.Join(wsRoot, fmt.Sprintf("%s-%s.zip", tgt.ID, stamp))
		// Never let a backup recurse into the backups root if a workspace lives
		// above it; zipDir skips any path inside `root`.
		n, err := zipDir(tgt.Dir, archive, root)
		if err != nil {
			res.Failures[tgt.ID] = err.Error()
			m.logger.Warn("backup workspace failed", "workspace", tgt.ID, "error", err)
			_ = os.Remove(archive) // drop a half-written archive
			continue
		}
		res.Archives = append(res.Archives, ArchiveInfo{
			WorkspaceID: tgt.ID, WorkspaceName: tgt.Name, Path: archive, Bytes: n,
		})
		prune(wsRoot, tgt.ID, cfg.Retain, m.logger)
	}

	res.Finished = time.Now()
	var errMsg string
	if len(res.Failures) > 0 {
		errMsg = fmt.Sprintf("%d workspace(s) failed", len(res.Failures))
	}
	m.recordRun(res.Finished, errMsg)
	m.logger.Info("backup pass complete",
		"archives", len(res.Archives), "failures", len(res.Failures), "dir", root)
	return res, nil
}

// Status is a lightweight snapshot of the manager state for the API.
type Status struct {
	Enabled       bool   `json:"enabled"`
	IntervalHours int    `json:"intervalHours"`
	Retain        int    `json:"retain"`
	Dir           string `json:"dir"`
	Running       bool   `json:"running"`
	LastRun       int64  `json:"lastRun"` // unix seconds; 0 = never
	LastError     string `json:"lastError,omitempty"`
}

// Status returns the current configuration + last-run info.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	var last int64
	if !m.lastRun.IsZero() {
		last = m.lastRun.Unix()
	}
	return Status{
		Enabled:       m.cfg.Enabled,
		IntervalHours: m.cfg.IntervalHours,
		Retain:        m.cfg.Retain,
		Dir:           m.resolveDir(m.cfg),
		Running:       m.running,
		LastRun:       last,
		LastError:     m.lastErr,
	}
}

func (m *Manager) recordRun(at time.Time, errMsg string) {
	m.mu.Lock()
	m.lastRun = at
	m.lastErr = errMsg
	m.mu.Unlock()
}

// resolveDir returns the absolute backups root, defaulting to <dataDir>/backups.
func (m *Manager) resolveDir(cfg Config) string {
	if strings.TrimSpace(cfg.Dir) != "" {
		return cfg.Dir
	}
	return filepath.Join(m.dataDir, "backups")
}

// prune keeps only the newest `retain` archives for a workspace, deleting older
// ones. Archives are matched by the "<id>-<stamp>.zip" prefix so unrelated files
// in the folder are left untouched.
func prune(dir, id string, retain int, logger *slog.Logger) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	prefix := id + "-"
	var archives []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".zip") {
			archives = append(archives, name)
		}
	}
	if len(archives) <= retain {
		return
	}
	// The "<id>-YYYYMMDD-HHMMSS.zip" name sorts chronologically as a string.
	sort.Strings(archives)
	for _, name := range archives[:len(archives)-retain] {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && logger != nil {
			logger.Warn("prune old backup failed", "file", name, "error", err)
		}
	}
}

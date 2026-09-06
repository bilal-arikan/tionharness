package db

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// ErrConflict is returned when a compare-and-swap write finds the row at a
// different revision than the caller expected. HTTP handlers map it to 409 so
// the client re-reads and retries on the fresh revision instead of clobbering
// a concurrent change.
var ErrConflict = errors.New("conflict: revision changed")

// trajectoryFile is the per-root-session sidecar holding the trajectory graph.
// It lives next to session.json so it is created, backed up and deleted with
// the session it belongs to.
const trajectoryFile = "trajectory.json"

// trajectoryIndexFile is the store-level listing of every trajectory
// (<store>/trajectories/index.json): id, root, template, status, revision.
// Listing and filtering read this and never open a sidecar; it is rewritten on
// every trajectory write and rebuilt from the sidecars if it is ever corrupt.
const trajectoryIndexFile = "index.json"

// Trajectory change ops delivered to the trajectory hook.
const (
	TrajectoryOpCreate = "create"
	TrajectoryOpUpdate = "update"
	TrajectoryOpDelete = "delete"
)

// TrajectoryChangeEvent describes one trajectory mutation. Trajectory is the
// graph AFTER the change (for delete, as it was; may be zero-valued when only
// the index entry could be recovered).
type TrajectoryChangeEvent struct {
	TrajectoryID  string
	RootSessionID string
	Op            string
	Trajectory    Trajectory
}

// TrajectoryChangeFn observes trajectory mutations. Same contract as the
// session/board hooks: called after every lock is released, must return
// promptly, may re-enter the DB.
type TrajectoryChangeFn func(ev TrajectoryChangeEvent)

// SetTrajectoryHook registers (or clears, with nil) the trajectory observer.
func (d *DB) SetTrajectoryHook(fn TrajectoryChangeFn) {
	d.trajHookMu.Lock()
	d.trajHook = fn
	d.trajHookMu.Unlock()
}

func (d *DB) fireTrajectoryHook(ev TrajectoryChangeEvent) {
	d.trajHookMu.RLock()
	fn := d.trajHook
	d.trajHookMu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

// ---- locking ----
//
// LOCK ORDER, mandatory: per-root trajectory lock FIRST, then trajIndexMu, then
// d.mu (only ever taken indirectly, via GetSession-style readers). Nothing may
// take a trajectory lock while holding d.mu or trajIndexMu. deleteSession honours
// this by dropping the index entry AFTER its own locks are released.

// trajLock returns the mutex serialising writers of one root session's
// trajectory sidecar. Same shape as transcriptLock; dropped with the session.
func (d *DB) trajLock(rootSessionID string) *sync.Mutex {
	d.trajLocksMu.Lock()
	defer d.trajLocksMu.Unlock()
	if d.trajLocks == nil {
		d.trajLocks = map[string]*sync.Mutex{}
	}
	mu, ok := d.trajLocks[rootSessionID]
	if !ok {
		mu = &sync.Mutex{}
		d.trajLocks[rootSessionID] = mu
	}
	return mu
}

func (d *DB) dropTrajLock(rootSessionID string) {
	d.trajLocksMu.Lock()
	delete(d.trajLocks, rootSessionID)
	d.trajLocksMu.Unlock()
}

func (d *DB) trajectorySidecar(rootSessionID string) Sidecar[Trajectory] {
	return newSidecar[Trajectory](d, rootSessionID, d.dir(dirSessions, rootSessionID, trajectoryFile))
}

// ---- index ----

// loadTrajectoryIndex reads trajectories/index.json at boot. A missing file is a
// fresh store (no trajectory was ever written — every write persists the index).
// A corrupt file is quarantined and the index rebuilt from the sidecars, so a
// bad index can never hide existing trajectories.
func (d *DB) loadTrajectoryIndex() error {
	d.trajIndex = map[string]TrajectoryIndexEntry{}
	sc := newSidecar[map[string]TrajectoryIndexEntry](d, "", d.dir(dirTrajectories, trajectoryIndexFile))
	idx, ok, err := sc.Load()
	if err != nil {
		if !IsSidecarCorrupt(err) {
			return err
		}
		slog.Warn("trajectory index corrupt; rebuilding from sidecars", "component", "db", "error", err)
		return d.rebuildTrajectoryIndexLocked()
	}
	if ok && idx != nil {
		d.trajIndex = idx
	}
	return nil
}

// RebuildTrajectoryIndex rescans every session directory for a trajectory
// sidecar and rewrites the index from what it finds. Repair tool for an index
// that was lost or hand-edited; the boot path calls it for a corrupt index.
func (d *DB) RebuildTrajectoryIndex() error {
	d.trajIndexMu.Lock()
	defer d.trajIndexMu.Unlock()
	return d.rebuildTrajectoryIndexLocked()
}

// rebuildTrajectoryIndexLocked scans <store>/sessions/*/trajectory.json.
// Caller holds trajIndexMu (or is boot).
func (d *DB) rebuildTrajectoryIndexLocked() error {
	entries, err := os.ReadDir(d.dir(dirSessions))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	idx := map[string]TrajectoryIndexEntry{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sc := d.trajectorySidecar(e.Name())
		if !sc.Exists() {
			continue
		}
		t, ok, lerr := sc.Load()
		if lerr != nil || !ok {
			// A corrupt sidecar was quarantined by Load; skip it.
			continue
		}
		idx[t.ID] = t.IndexEntry()
	}
	d.trajIndex = idx
	return d.persistTrajectoryIndexLocked()
}

// persistTrajectoryIndexLocked writes the index. Caller holds trajIndexMu.
func (d *DB) persistTrajectoryIndexLocked() error {
	// Trajectory nodes are part of the Explorer graph, so an index change must
	// invalidate the caches keyed on MutationGen like any entity write.
	d.markMutatedLocked()
	return atomicWriteJSON(d.dir(dirTrajectories, trajectoryIndexFile), d.trajIndex)
}

func (d *DB) indexPut(entry TrajectoryIndexEntry) error {
	d.trajIndexMu.Lock()
	defer d.trajIndexMu.Unlock()
	d.trajIndex[entry.ID] = entry
	return d.persistTrajectoryIndexLocked()
}

func (d *DB) indexDelete(id string) (TrajectoryIndexEntry, bool, error) {
	d.trajIndexMu.Lock()
	defer d.trajIndexMu.Unlock()
	entry, ok := d.trajIndex[id]
	if !ok {
		return TrajectoryIndexEntry{}, false, nil
	}
	delete(d.trajIndex, id)
	return entry, true, d.persistTrajectoryIndexLocked()
}

func (d *DB) indexGet(id string) (TrajectoryIndexEntry, bool) {
	d.trajIndexMu.RLock()
	defer d.trajIndexMu.RUnlock()
	entry, ok := d.trajIndex[id]
	return entry, ok
}

func (d *DB) indexByRoot(rootSessionID string) (TrajectoryIndexEntry, bool) {
	d.trajIndexMu.RLock()
	defer d.trajIndexMu.RUnlock()
	for _, e := range d.trajIndex {
		if e.RootSessionID == rootSessionID {
			return e, true
		}
	}
	return TrajectoryIndexEntry{}, false
}

// ---- CRUD ----

// CreateTrajectory persists a new trajectory for its root session. Exactly one
// trajectory per root: a second create for the same root returns ErrConflict
// (use UpdateTrajectory to change the existing one). The root session must
// exist. ID, Revision (1) and timestamps are assigned here; Status defaults to
// planned; Nodes/Edges default to empty slices so the JSON always carries them.
func (d *DB) CreateTrajectory(ctx context.Context, t Trajectory) (Trajectory, error) {
	root := strings.TrimSpace(t.RootSessionID)
	if root == "" {
		return Trajectory{}, fmt.Errorf("trajectory requires rootSessionId")
	}
	if _, err := d.GetSession(ctx, root); err != nil {
		return Trajectory{}, fmt.Errorf("root session %q: %w", root, err)
	}
	mu := d.trajLock(root)
	mu.Lock()
	defer mu.Unlock()
	if _, exists := d.indexByRoot(root); exists {
		return Trajectory{}, fmt.Errorf("root session %q already has a trajectory: %w", root, ErrConflict)
	}
	if d.trajectorySidecar(root).Exists() {
		// Index lost the row but the file is there: refuse rather than overwrite;
		// RebuildTrajectoryIndex recovers it.
		return Trajectory{}, fmt.Errorf("root session %q has an unindexed trajectory sidecar (run RebuildTrajectoryIndex): %w", root, ErrConflict)
	}
	t.RootSessionID = root
	t.ID = d.nextID(idTrajectory)
	t.Revision = 1
	t.CreatedAt = now()
	t.UpdatedAt = t.CreatedAt
	if t.Status == "" {
		t.Status = TrajStatusPlanned
	}
	if t.Nodes == nil {
		t.Nodes = []TrajectoryNode{}
	}
	if t.Edges == nil {
		t.Edges = []TrajectoryEdge{}
	}
	if err := t.Validate(); err != nil {
		return Trajectory{}, err
	}
	if err := d.trajectorySidecar(root).Save(t); err != nil {
		return Trajectory{}, err
	}
	if err := d.indexPut(t.IndexEntry()); err != nil {
		return Trajectory{}, err
	}
	d.fireTrajectoryHook(TrajectoryChangeEvent{TrajectoryID: t.ID, RootSessionID: root, Op: TrajectoryOpCreate, Trajectory: t})
	return t, nil
}

// GetTrajectory loads a trajectory by id. ErrNotFound when unknown; a corrupt
// sidecar is quarantined, dropped from the index and reported once as
// *SidecarCorruptError (later reads say ErrNotFound).
func (d *DB) GetTrajectory(ctx context.Context, id string) (Trajectory, error) {
	entry, ok := d.indexGet(id)
	if !ok {
		return Trajectory{}, ErrNotFound
	}
	mu := d.trajLock(entry.RootSessionID)
	mu.Lock()
	defer mu.Unlock()
	return d.loadTrajectoryLocked(entry)
}

// GetTrajectoryByRoot loads the trajectory owned by a root session.
func (d *DB) GetTrajectoryByRoot(ctx context.Context, rootSessionID string) (Trajectory, error) {
	entry, ok := d.indexByRoot(rootSessionID)
	if !ok {
		return Trajectory{}, ErrNotFound
	}
	mu := d.trajLock(entry.RootSessionID)
	mu.Lock()
	defer mu.Unlock()
	return d.loadTrajectoryLocked(entry)
}

// loadTrajectoryLocked reads the sidecar behind an index entry. Caller holds the
// root's trajectory lock. An absent or corrupt file drops the stale index row.
func (d *DB) loadTrajectoryLocked(entry TrajectoryIndexEntry) (Trajectory, error) {
	t, ok, err := d.trajectorySidecar(entry.RootSessionID).Load()
	if err != nil {
		if IsSidecarCorrupt(err) {
			_, _, _ = d.indexDelete(entry.ID)
		}
		return Trajectory{}, err
	}
	if !ok {
		_, _, _ = d.indexDelete(entry.ID)
		return Trajectory{}, ErrNotFound
	}
	return t, nil
}

// TrajectoryFilter narrows ListTrajectories. Empty fields match everything.
type TrajectoryFilter struct {
	RootSessionID string
	TemplateRef   string
	Status        string
	// Terminal, when non-nil, keeps only terminal (true) or live (false) rows.
	Terminal *bool
}

// ListTrajectories returns index rows matching f, newest first. It reads only
// the index — no sidecar is opened.
func (d *DB) ListTrajectories(ctx context.Context, f TrajectoryFilter) []TrajectoryIndexEntry {
	d.trajIndexMu.RLock()
	out := make([]TrajectoryIndexEntry, 0, len(d.trajIndex))
	for _, e := range d.trajIndex {
		if f.RootSessionID != "" && e.RootSessionID != f.RootSessionID {
			continue
		}
		if f.TemplateRef != "" && e.TemplateRef != f.TemplateRef {
			continue
		}
		if f.Status != "" && e.Status != f.Status {
			continue
		}
		if f.Terminal != nil && trajTerminalSet[e.Status] != *f.Terminal {
			continue
		}
		out = append(out, e)
	}
	d.trajIndexMu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt != out[j].UpdatedAt {
			return out[i].UpdatedAt > out[j].UpdatedAt
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// UpdateTrajectory applies fn to the current graph under the root's lock and
// persists the result with Revision+1.
//
// expectedRev is the compare-and-swap guard: when non-zero the update is
// refused with ErrConflict unless the stored revision matches, which is what a
// UI or agent edit must pass (it echoes the revision it rendered). Runtime
// observers that only APPEND observed facts (a spawn, a report, a fire) pass 0:
// they hold the lock, so they never lose a write, and their append is valid on
// whatever revision is current. fn may return an error to abort without a
// write; the graph is validated after fn and a structurally invalid result is
// rejected unwritten.
func (d *DB) UpdateTrajectory(ctx context.Context, id string, expectedRev uint64, fn func(*Trajectory) error) (Trajectory, error) {
	entry, ok := d.indexGet(id)
	if !ok {
		return Trajectory{}, ErrNotFound
	}
	mu := d.trajLock(entry.RootSessionID)
	mu.Lock()
	defer mu.Unlock()
	t, err := d.loadTrajectoryLocked(entry)
	if err != nil {
		return Trajectory{}, err
	}
	if expectedRev != 0 && t.Revision != expectedRev {
		return Trajectory{}, fmt.Errorf("trajectory %s is at revision %d, caller expected %d: %w", id, t.Revision, expectedRev, ErrConflict)
	}
	prevRev := t.Revision
	if err := fn(&t); err != nil {
		return Trajectory{}, err
	}
	// Identity and bookkeeping are the store's, whatever fn did to them.
	t.ID = entry.ID
	t.RootSessionID = entry.RootSessionID
	t.Revision = prevRev + 1
	t.UpdatedAt = now()
	if t.CreatedAt == 0 {
		t.CreatedAt = entry.CreatedAt
	}
	if t.Nodes == nil {
		t.Nodes = []TrajectoryNode{}
	}
	if t.Edges == nil {
		t.Edges = []TrajectoryEdge{}
	}
	if err := t.Validate(); err != nil {
		return Trajectory{}, err
	}
	if err := d.trajectorySidecar(t.RootSessionID).Save(t); err != nil {
		return Trajectory{}, err
	}
	if err := d.indexPut(t.IndexEntry()); err != nil {
		return Trajectory{}, err
	}
	d.fireTrajectoryHook(TrajectoryChangeEvent{TrajectoryID: t.ID, RootSessionID: t.RootSessionID, Op: TrajectoryOpUpdate, Trajectory: t})
	return t, nil
}

// DeleteTrajectory removes a trajectory's sidecar and index row. ErrNotFound
// when unknown. The root session itself is untouched.
func (d *DB) DeleteTrajectory(ctx context.Context, id string) error {
	entry, ok := d.indexGet(id)
	if !ok {
		return ErrNotFound
	}
	mu := d.trajLock(entry.RootSessionID)
	mu.Lock()
	t, _, _ := d.trajectorySidecar(entry.RootSessionID).Load()
	if err := d.trajectorySidecar(entry.RootSessionID).Clear(); err != nil {
		mu.Unlock()
		return err
	}
	_, _, err := d.indexDelete(id)
	mu.Unlock()
	if err != nil {
		return err
	}
	d.fireTrajectoryHook(TrajectoryChangeEvent{TrajectoryID: id, RootSessionID: entry.RootSessionID, Op: TrajectoryOpDelete, Trajectory: t})
	return nil
}

// dropTrajectoryForRoot forgets the trajectory of a deleted root session: the
// sidecar died with the session directory, so only the index row and the lock
// remain. Called by deleteSession AFTER its locks are released (lock order).
func (d *DB) dropTrajectoryForRoot(rootSessionID string) {
	entry, ok := d.indexByRoot(rootSessionID)
	if !ok {
		d.dropTrajLock(rootSessionID)
		return
	}
	mu := d.trajLock(rootSessionID)
	mu.Lock()
	_, _, err := d.indexDelete(entry.ID)
	mu.Unlock()
	d.dropTrajLock(rootSessionID)
	if err != nil {
		slog.Warn("drop trajectory index for deleted session failed", "component", "db",
			"session", rootSessionID, "trajectory", entry.ID, "error", err)
	}
	d.fireTrajectoryHook(TrajectoryChangeEvent{TrajectoryID: entry.ID, RootSessionID: rootSessionID, Op: TrajectoryOpDelete})
}

// TrajectoryPath returns the absolute sidecar path for a root session (for
// diagnostics / "open file" affordances). It does not check existence.
func (d *DB) TrajectoryPath(rootSessionID string) string {
	return filepath.Clean(d.trajectorySidecar(rootSessionID).Path())
}

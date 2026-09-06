package workspace

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// seedLiveEntry registers a workspace in the live map without opening it. Every
// test below fails its persist before the teardown block is reached, so the nil
// Scheduler/DB/Runtime of this stand-in is never touched — and that is itself
// part of the contract: a delete whose registry write failed must not have torn
// anything down.
func seedLiveEntry(t *testing.T, m *Manager, id, name string, createdAt int64) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), id)
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("seed workspace dir: %v", err)
	}
	m.workspaces[id] = &Workspace{
		Meta:    Meta{ID: id, Name: name, CreatedAt: createdAt, Path: dir},
		DataDir: dir,
	}
	m.order = append(m.order, id)
	return dir
}

// blockRegistryPath forces every later persist() to fail, the same way as
// TestPersistRemovesTempOnFailure: a non-empty DIRECTORY at the registry path can
// never be replaced by a rename.
func blockRegistryPath(t *testing.T, m *Manager) {
	t.Helper()
	if err := os.MkdirAll(m.metaPath(), 0o755); err != nil {
		t.Fatalf("seed blocking dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.metaPath(), "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}
}

// TestDeleteLiveRollsBackOnPersistFailure: the live delete used to tear the
// workspace down and erase its directory FIRST and persist afterwards, so a failed
// registry write left the worst possible state — the workspace gone from memory
// and from disk, but still listed in workspaces.json, i.e. a degraded entry
// pointing at nothing on the next boot. Persisting inside the same hold as the
// removal means a failure changes nothing: the workspace is still live, still
// listed, and its data is still there.
func TestDeleteLiveRollsBackOnPersistFailure(t *testing.T) {
	m := testManager(t)
	firstDir := seedLiveEntry(t, m, "WS1", "first", 10)
	secondDir := seedLiveEntry(t, m, "WS2", "second", 50)
	blockRegistryPath(t, m)

	if err := m.Delete("WS2"); err == nil {
		t.Fatal("delete must fail when the registry cannot be persisted")
	}

	got := m.List()
	if len(got) != 2 || got[0].ID != "WS1" || got[1].ID != "WS2" {
		t.Fatalf("List() = %+v, want WS1 then WS2 still live", got)
	}
	if _, err := os.Stat(secondDir); err != nil {
		t.Fatalf("workspace dir removed despite the failed delete: %v", err)
	}
	if _, err := os.Stat(firstDir); err != nil {
		t.Fatalf("the untouched workspace's dir went missing: %v", err)
	}
	// The rollback must not leave the directory claimed either, or the folder
	// would stay unattachable for the rest of the process's life.
	m.mu.RLock()
	pending := len(m.pendingRemoval)
	m.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("pendingRemoval = %d entries after a rolled-back delete, want 0", pending)
	}
}

// TestDeleteLiveRollbackIsNotObservable is TestDeleteDegradedRollbackIsNotObservable
// for the live path: the removal, the registry write and the rollback happen in ONE
// write-lock hold, so a concurrent reader never sees a workspace that was never
// deleted as gone. Readers run for the whole (failing) delete and every observation
// must still contain WS2.
func TestDeleteLiveRollbackIsNotObservable(t *testing.T) {
	m := testManager(t)
	seedLiveEntry(t, m, "WS1", "first", 10)
	dir := seedLiveEntry(t, m, "WS2", "second", 50)
	blockRegistryPath(t, m)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var misses atomic.Int64
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				found := false
				for _, meta := range m.List() {
					if meta.ID == "WS2" {
						found = true
					}
				}
				if !found {
					misses.Add(1)
				}
			}
		}()
	}

	for i := 0; i < 50; i++ {
		if err := m.Delete("WS2"); err == nil {
			close(stop)
			wg.Wait()
			t.Fatal("delete must fail when the registry cannot be persisted")
		}
	}
	close(stop)
	wg.Wait()

	if n := misses.Load(); n != 0 {
		t.Fatalf("List observed WS2 as deleted %d times during a rolled-back delete", n)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace dir removed despite the failed delete: %v", err)
	}
}

// TestDeleteLiveRemovesRegistryEntryAndDir drives the happy path through a real,
// fully opened workspace — the ordering change moved persist() ahead of the
// teardown, so the guard that matters is that a real Scheduler/DB/MCP handle is
// still stopped and the directory still erased once the registry write succeeded.
func TestDeleteLiveRemovesRegistryEntryAndDir(t *testing.T) {
	m := liveTestManager(t)
	ws, err := m.Create("doomed", "", "test")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	dataDir := ws.DataDir

	if err := m.Delete(ws.ID); err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if got := m.List(); len(got) != 0 {
		t.Fatalf("List() = %+v, want no workspaces left", got)
	}
	metas, err := m.loadMetas()
	if err != nil {
		t.Fatalf("loadMetas: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("registry after delete = %+v, want it empty", metas)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("workspace dir survived the delete: %v", err)
	}
	m.mu.RLock()
	pending := len(m.pendingRemoval)
	m.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("pendingRemoval = %d entries after the delete returned, want 0", pending)
	}
}

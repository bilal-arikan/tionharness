package workspace

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// TestDeleteDegradedWorkspace: a workspace that failed to open is absent from the
// live map, so Delete used to answer "workspace not found" and the broken record
// could never be purged from workspaces.json. Deleting one must drop the registry
// entry AND its data directory.
func TestDeleteDegradedWorkspace(t *testing.T) {
	m := testManager(t)
	live := Meta{ID: "WS1", Name: "live", CreatedAt: 100}
	m.workspaces[live.ID] = &Workspace{Meta: live}
	m.order = append(m.order, live.ID)

	dir := filepath.Join(t.TempDir(), "ws2")
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("seed workspace dir: %v", err)
	}
	broken := Meta{ID: "WS2", Name: "broken", CreatedAt: 50, Path: dir}
	m.markDegraded(broken, errors.New("store is corrupt"))

	if err := m.Delete(broken.ID); err != nil {
		t.Fatalf("delete degraded workspace: %v", err)
	}
	if len(m.degraded) != 0 {
		t.Fatalf("degraded list still holds %+v", m.degraded)
	}
	metas, err := m.loadMetas()
	if err != nil {
		t.Fatalf("loadMetas: %v", err)
	}
	if len(metas) != 1 || metas[0].ID != live.ID {
		t.Fatalf("registry after delete = %+v, want only %s", metas, live.ID)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("degraded workspace dir survived the delete: %v", err)
	}
	// An id that is in neither list is still an error.
	if err := m.Delete("WS9"); err == nil {
		t.Fatal("deleting an unknown workspace must fail")
	}
}

// TestDeleteDegradedRollsBackOnPersistFailure: the registry entry is the thing the
// delete is about, so a failed persist must change nothing. Dropping the record
// from memory only would leave workspaces.json holding a workspace the manager no
// longer knows — and the next successful persist (a Create, say) would erase it
// without the user ever asking. The persist failure is forced the same way as in
// TestPersistRemovesTempOnFailure: a non-empty DIRECTORY at the registry path, which
// no rename can replace.
func TestDeleteDegradedRollsBackOnPersistFailure(t *testing.T) {
	m := testManager(t)
	m.markDegraded(Meta{ID: "WS1", Name: "first", CreatedAt: 10}, errors.New("boom"))
	dir := filepath.Join(t.TempDir(), "ws2")
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("seed workspace dir: %v", err)
	}
	m.markDegraded(Meta{ID: "WS2", Name: "broken", CreatedAt: 50, Path: dir}, errors.New("store is corrupt"))

	if err := os.MkdirAll(m.metaPath(), 0o755); err != nil {
		t.Fatalf("seed blocking dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.metaPath(), "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}

	if err := m.Delete("WS2"); err == nil {
		t.Fatal("delete must fail when the registry cannot be persisted")
	}

	entries := m.ListWithDegraded()
	if len(entries) != 2 || entries[0].ID != "WS1" || entries[1].ID != "WS2" {
		t.Fatalf("ListWithDegraded() = %+v, want WS1 then WS2 still registered", entries)
	}
	if !entries[1].Degraded || entries[1].Reason != "store is corrupt" {
		t.Fatalf("restored entry lost its degraded state: %+v", entries[1])
	}
	// The data directory must survive too: nothing was deleted.
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace dir removed despite the failed delete: %v", err)
	}
}

// TestDeleteDegradedRollbackIsNotObservable: the rollback of a failed delete must
// happen in the SAME lock hold as the removal, so no reader ever sees the entry
// missing. While persist() locked internally, deleteDegraded had to drop the lock
// around it — a concurrent ListWithDegraded could then observe a workspace that
// was never actually deleted as gone. Readers run for the whole (failing) delete
// and every observation must still contain WS2.
func TestDeleteDegradedRollbackIsNotObservable(t *testing.T) {
	m := testManager(t)
	m.markDegraded(Meta{ID: "WS1", Name: "first", CreatedAt: 10}, errors.New("boom"))
	dir := filepath.Join(t.TempDir(), "ws2")
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("seed workspace dir: %v", err)
	}
	m.markDegraded(Meta{ID: "WS2", Name: "broken", CreatedAt: 50, Path: dir}, errors.New("store is corrupt"))

	// Force the persist to fail: a non-empty directory at the registry path can
	// never be replaced by a rename.
	if err := os.MkdirAll(m.metaPath(), 0o755); err != nil {
		t.Fatalf("seed blocking dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.metaPath(), "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}

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
				for _, e := range m.ListWithDegraded() {
					if e.ID == "WS2" {
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
		t.Fatalf("ListWithDegraded observed WS2 as deleted %d times during a rolled-back delete", n)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("workspace dir removed despite the failed delete: %v", err)
	}
}

// TestListWithDegradedFlagsBrokenWorkspaces: a degraded workspace is invisible in
// List() (every caller pairs a Meta with a live handle), so the registry view the
// user sees must come from ListWithDegraded and must say WHY it is broken.
func TestListWithDegradedFlagsBrokenWorkspaces(t *testing.T) {
	m := testManager(t)
	live := Meta{ID: "WS1", Name: "live", CreatedAt: 100}
	m.workspaces[live.ID] = &Workspace{Meta: live}
	m.order = append(m.order, live.ID)
	m.markDegraded(Meta{ID: "WS2", Name: "broken", CreatedAt: 50}, errors.New("store is corrupt"))

	if got := m.List(); len(got) != 1 || got[0].ID != "WS1" {
		t.Fatalf("List() = %+v, want only the live workspace", got)
	}

	entries := m.ListWithDegraded()
	if len(entries) != 2 {
		t.Fatalf("ListWithDegraded() = %+v, want 2 entries", entries)
	}
	// Creation order: the degraded one is older, so it comes first.
	if entries[0].ID != "WS2" || !entries[0].Degraded {
		t.Fatalf("degraded entry not flagged: %+v", entries[0])
	}
	if entries[0].Reason != "store is corrupt" {
		t.Fatalf("degraded reason = %q, want the open error", entries[0].Reason)
	}
	if entries[1].ID != "WS1" || entries[1].Degraded {
		t.Fatalf("live entry mis-flagged: %+v", entries[1])
	}
}

// TestPersistRemovesTempOnFailure: a failed write or rename must not leave a
// workspaces.json.tmp next to the registry. The failure is forced by making the
// registry path a non-empty DIRECTORY, which no rename can replace (on Windows
// and POSIX alike) while the temp write itself still succeeds.
func TestPersistRemovesTempOnFailure(t *testing.T) {
	m := testManager(t)
	m.workspaces["WS1"] = &Workspace{Meta: Meta{ID: "WS1", Name: "a", CreatedAt: 1}}
	m.order = append(m.order, "WS1")

	if err := os.MkdirAll(m.metaPath(), 0o755); err != nil {
		t.Fatalf("seed blocking dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.metaPath(), "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed blocking file: %v", err)
	}

	if err := m.persist(m.registryMetas()); err == nil {
		t.Fatal("persist must fail when the registry path cannot be replaced")
	}
	if _, err := os.Stat(m.metaPath() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp registry file left behind after a failed persist: %v", err)
	}
}

// TestPersistSyncsBeforeRename: the happy path still produces a parseable
// registry through the fsync'd write (guards the os.WriteFile → OpenFile+Sync
// swap from silently writing nothing).
func TestPersistSyncsBeforeRename(t *testing.T) {
	m := testManager(t)
	m.workspaces["WS1"] = &Workspace{Meta: Meta{ID: "WS1", Name: "a", CreatedAt: 1}}
	m.order = append(m.order, "WS1")
	if err := m.persist(m.registryMetas()); err != nil {
		t.Fatalf("persist: %v", err)
	}
	data, err := os.ReadFile(m.metaPath())
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var metas []Meta
	if err := json.Unmarshal(data, &metas); err != nil {
		t.Fatalf("registry does not parse: %v", err)
	}
	if len(metas) != 1 || metas[0].ID != "WS1" {
		t.Fatalf("registry = %+v, want one WS1 entry", metas)
	}
}

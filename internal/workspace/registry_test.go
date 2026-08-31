package workspace

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	return &Manager{
		rootDir:    t.TempDir(),
		workspaces: make(map[string]*Workspace),
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// TestPersistKeepsDegradedWorkspaces: a workspace that failed to open keeps its
// entry in workspaces.json. Dropping it made the next Create/Delete erase the
// only pointer to a data directory that is still fully intact on disk.
func TestPersistKeepsDegradedWorkspaces(t *testing.T) {
	m := testManager(t)
	live := Meta{ID: "WS1", Name: "live", CreatedAt: 100}
	broken := Meta{ID: "WS2", Name: "broken", CreatedAt: 50, Path: `C:\somewhere\ws2`}
	m.workspaces[live.ID] = &Workspace{Meta: live}
	m.order = append(m.order, live.ID)
	m.markDegraded(broken, errors.New("store is corrupt"))
	m.markDegraded(broken, errors.New("store is corrupt")) // idempotent

	if err := m.persist(); err != nil {
		t.Fatalf("persist: %v", err)
	}
	metas, err := m.loadMetas()
	if err != nil {
		t.Fatalf("loadMetas: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("registry has %d entries, want 2: %+v", len(metas), metas)
	}
	// Creation order: the degraded one is older, so it comes first.
	if metas[0].ID != "WS2" || metas[1].ID != "WS1" {
		t.Fatalf("registry order = %s,%s, want WS2,WS1", metas[0].ID, metas[1].ID)
	}
	if metas[0].Path != broken.Path {
		t.Fatalf("degraded meta lost its path: %+v", metas[0])
	}
}

// TestPersistIsAtomic: the registry is written through a temp file + rename, so a
// crash mid-write cannot leave a truncated workspaces.json (the only pointer to
// every workspace's data). The observable trace is that no .tmp survives and the
// document always parses.
func TestPersistIsAtomic(t *testing.T) {
	m := testManager(t)
	m.workspaces["WS1"] = &Workspace{Meta: Meta{ID: "WS1", Name: "a", CreatedAt: 1}}
	m.order = append(m.order, "WS1")
	if err := m.persist(); err != nil {
		t.Fatalf("persist: %v", err)
	}
	if _, err := os.Stat(m.metaPath() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp registry file left behind: %v", err)
	}
	data, err := os.ReadFile(m.metaPath())
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	var metas []Meta
	if err := json.Unmarshal(data, &metas); err != nil {
		t.Fatalf("registry does not parse: %v", err)
	}
}

// TestLoadMetasCorruptRegistryIsFatal: a corrupt workspaces.json must be an
// error, never an empty list — reading it as "no workspaces" would let the next
// persist() overwrite it and erase every workspace the user has.
func TestLoadMetasCorruptRegistryIsFatal(t *testing.T) {
	m := testManager(t)
	if err := os.WriteFile(m.metaPath(), []byte(`[{"id":"WS1"`), 0o644); err != nil {
		t.Fatalf("write registry: %v", err)
	}
	metas, err := m.loadMetas()
	if err == nil {
		t.Fatalf("corrupt registry loaded without error: %+v", metas)
	}
}

// TestLoadMetasMissingRegistryIsEmpty: only a MISSING file is a legitimately
// empty registry (first run).
func TestLoadMetasMissingRegistryIsEmpty(t *testing.T) {
	m := testManager(t)
	metas, err := m.loadMetas()
	if err != nil {
		t.Fatalf("missing registry must not be an error: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("want empty registry, got %+v", metas)
	}
	if _, err := os.Stat(filepath.Join(m.rootDir, "workspaces.json")); !os.IsNotExist(err) {
		t.Fatalf("loadMetas must not create the registry: %v", err)
	}
}

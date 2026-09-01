package workspace

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/config"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// liveTestManager builds a fully wired manager (unlike testManager, whose zero
// registry/bus cannot open a workspace). The removal-window tests need Attach to
// run all the way through open(), because "was the folder adopted" is the whole
// question.
func liveTestManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	cipher, err := config.LoadSecret(root)
	if err != nil {
		t.Fatalf("load cipher: %v", err)
	}
	m, err := NewManager(
		root,
		providers.NewRegistry(),
		agent.NewTunables(),
		cipher,
		events.NewBus(),
		logbuf.New(16),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(m.Close)
	return m
}

// seedRemovalWindowDir builds a workspace-looking directory whose removal takes
// long enough to observe. The bulk files live under a name that sorts BEFORE
// "store", so os.RemoveAll (which walks the directory in readdir order) is still
// busy with them while store/ — the only thing Attach validates — is on disk.
// That is what makes the window in the test below a real one instead of a
// nanosecond.
func seedRemovalWindowDir(t *testing.T, files int) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ws-target")
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("seed store dir: %v", err)
	}
	bulk := filepath.Join(dir, "aaa-bulk")
	if err := os.MkdirAll(bulk, 0o755); err != nil {
		t.Fatalf("seed bulk dir: %v", err)
	}
	for i := 0; i < files; i++ {
		p := filepath.Join(bulk, fmt.Sprintf("f%04d.bin", i))
		if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
			t.Fatalf("seed bulk file: %v", err)
		}
	}
	return dir
}

// TestDeleteMarksAndClearsRemovalOnTheProductionPath drives the pendingRemoval
// guard through the REAL delete instead of setting the mark by hand: it proves
// that Delete/deleteDegraded actually marks the directory it is about to erase,
// and that the mark is actually lifted afterwards.
//
// Part (a) is a race test in the same style as
// TestDeleteDegradedRollbackIsNotObservable: hammering Attach from four
// goroutines for the whole duration of a real delete. Every single one must be
// rejected — an Attach that succeeds has adopted a folder whose files the
// in-flight os.RemoveAll is deleting. Nothing in the production code is slowed
// down for this; the window is widened only by the seeded data (see
// seedRemovalWindowDir).
//
// Part (b) runs strictly AFTER the delete has returned and measures the
// contract, not the window: re-creating store/ and attaching the same path must
// now SUCCEED. It cannot succeed unless endRemoval really ran, so this is the
// external observation of "the mark is gone" — no internal field is read.
func TestDeleteMarksAndClearsRemovalOnTheProductionPath(t *testing.T) {
	m := liveTestManager(t)
	dir := seedRemovalWindowDir(t, 3000)
	m.markDegraded(Meta{ID: "WSX", Name: "broken", CreatedAt: 50, Path: dir}, errors.New("store is corrupt"))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var adopted atomic.Int64
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
				if _, err := m.Attach(dir); err == nil {
					adopted.Add(1)
					return
				}
			}
		}()
	}

	// The production entry point: Delete falls through to deleteDegraded for a
	// workspace that is registered but not live.
	delErr := m.Delete("WSX")
	close(stop)
	wg.Wait()

	// Checked before delErr on purpose: an adopted folder makes the removal fail
	// too (the new workspace's DB writes files back into the tree being erased),
	// and the adoption is the finding — the removal error is its symptom.
	if n := adopted.Load(); n != 0 {
		t.Fatalf("Attach adopted the folder %d times while it was being erased", n)
	}
	if delErr != nil {
		t.Fatalf("delete degraded workspace: %v", delErr)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("workspace dir survived the delete: %v", err)
	}

	// (b) The delete has returned, so the mark must be gone and the same path
	// must be attachable again.
	if err := os.MkdirAll(filepath.Join(dir, "store"), 0o755); err != nil {
		t.Fatalf("re-seed store dir: %v", err)
	}
	if _, err := m.Attach(dir); err != nil {
		t.Fatalf("attaching the folder after the delete finished must succeed, got: %v", err)
	}
}

// TestRestoreFromArchiveRejectsAttachWhileSwapping: RestoreFromArchive drops the
// registry entry under the lock and rewrites the directory's content outside it
// (os.RemoveAll + rename per top-level entry). That is the same window Delete
// has — nothing points at the folder, store/ is still on disk, so an Attach
// would adopt it, the swap would then rip the store out from under the new
// workspace, and open() would finally register the SAME directory a second time
// under the original id.
//
// No timing is involved here: extract is an injected callback that runs inside
// the window by construction, so the Attach below is guaranteed to land there.
func TestRestoreFromArchiveRejectsAttachWhileSwapping(t *testing.T) {
	m := liveTestManager(t)
	ws, err := m.Create("restore-target", "", "test")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	dataDir := ws.DataDir

	var attachErr error
	var attached bool
	extract := func(_, dst string) error {
		// Inside the window: the registry entry is gone and the live content is
		// about to be replaced.
		if _, err := m.Attach(dataDir); err == nil {
			attached = true
		} else {
			attachErr = err
		}
		if err := os.MkdirAll(filepath.Join(dst, "store"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, "restored.txt"), []byte("ok"), 0o644)
	}

	if err := m.RestoreFromArchive(ws.ID, "archive.zip", extract); err != nil {
		t.Fatalf("restore from archive: %v", err)
	}
	if attached {
		t.Fatal("Attach adopted the workspace folder while its content was being replaced")
	}
	if attachErr == nil {
		t.Fatal("extract hook never ran, so nothing was observed inside the window")
	}
	if _, err := os.Stat(filepath.Join(dataDir, "restored.txt")); err != nil {
		t.Fatalf("restored content missing, the swap did not happen: %v", err)
	}
	// The restore has returned: the mark must not outlive it, or the folder
	// would stay unattachable forever.
	m.mu.RLock()
	pending := len(m.pendingRemoval)
	m.mu.RUnlock()
	if pending != 0 {
		t.Fatalf("pendingRemoval = %d entries after the restore returned, want 0", pending)
	}
}

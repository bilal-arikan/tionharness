package providers

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeResumeHome creates the scoped home for scope with one rollout per thread,
// every file and folder dated at mtime.
func makeResumeHome(t *testing.T, base, scope string, mtime time.Time, threads ...string) string {
	t.Helper()
	dir := codexResumeHome(base, scope)
	rollouts := filepath.Join(dir, "sessions", "2026", "10", "18")
	if err := os.MkdirAll(rollouts, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state_5.sqlite"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range threads {
		if err := os.WriteFile(filepath.Join(rollouts, "rollout-2026-10-18T12-00-00-"+id+".jsonl"), []byte("{}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ageTree(t, dir, mtime)
	return dir
}

func ageTree(t *testing.T, dir string, mtime time.Time) {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		paths = append(paths, path)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Deepest first, so dating a file does not refresh its folder afterwards.
	for i := len(paths) - 1; i >= 0; i-- {
		if err := os.Chtimes(paths[i], mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestPruneCodexResumeHomesRemovesOnlyOrphans(t *testing.T) {
	base := t.TempDir()
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	liveThread := "019a0000-0000-7000-8000-000000000001"
	kept := makeResumeHome(t, base, "live", old, "019a0000-0000-7000-8000-00000000dead", liveThread)
	orphan := makeResumeHome(t, base, "orphan", old, "019a0000-0000-7000-8000-00000000beef")
	empty := makeResumeHome(t, base, "never-finished", old)
	young := makeResumeHome(t, base, "first-turn", now.Add(-time.Hour), "019a0000-0000-7000-8000-00000000cafe")
	stray := filepath.Join(base, codexResumeHomesDir, "notes")
	if err := os.MkdirAll(stray, 0o700); err != nil {
		t.Fatal(err)
	}
	ageTree(t, stray, old)

	live := map[string]struct{}{liveThread: {}}
	res, err := PruneCodexResumeHomes(base, live, CodexResumeHomeGrace, now)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 4 || res.Removed != 2 || res.FreedBytes <= 0 {
		t.Fatalf("result = %+v, want 4 scanned, 2 removed, bytes freed", res)
	}
	for _, dir := range []string{kept, young, stray} {
		if !exists(dir) {
			t.Errorf("%s was removed", dir)
		}
	}
	for _, dir := range []string{orphan, empty} {
		if exists(dir) {
			t.Errorf("%s survived the sweep", dir)
		}
	}
}

func TestPruneCodexResumeHomesRecentStateKeepsHome(t *testing.T) {
	base := t.TempDir()
	now := time.Now()
	dir := makeResumeHome(t, base, "resumed", now.Add(-72*time.Hour), "019a0000-0000-7000-8000-000000000002")
	// A resumed turn writes the sqlite state and WAL before any new rollout.
	if err := os.Chtimes(filepath.Join(dir, "state_5.sqlite"), now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := PruneCodexResumeHomes(base, nil, CodexResumeHomeGrace, now); err != nil {
		t.Fatal(err)
	}
	if !exists(dir) {
		t.Fatal("home written within the grace period was removed")
	}
}

func TestPruneCodexResumeHomesSkipsHomeHeldByTurn(t *testing.T) {
	base := t.TempDir()
	now := time.Now()
	held, release, err := prepareCodexTurnHome(base, "running")
	if err != nil {
		t.Fatal(err)
	}
	ageTree(t, held, now.Add(-48*time.Hour))

	if _, err := PruneCodexResumeHomes(base, nil, CodexResumeHomeGrace, now); err != nil {
		t.Fatal(err)
	}
	if !exists(held) {
		t.Fatal("home of a running turn was removed")
	}
	release()
	release() // idempotent: a double release must not free another turn's hold
	if _, err := PruneCodexResumeHomes(base, nil, CodexResumeHomeGrace, now); err != nil {
		t.Fatal(err)
	}
	if exists(held) {
		t.Fatal("released orphan home survived the sweep")
	}
}

func TestPruneCodexResumeHomesFinishesInterruptedDelete(t *testing.T) {
	base := t.TempDir()
	trash := filepath.Join(base, codexResumeHomesDir, codexResumeTrashPrefix+"abc")
	if err := os.MkdirAll(trash, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trash, "cache.bin"), make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := PruneCodexResumeHomes(base, nil, CodexResumeHomeGrace, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if exists(trash) || res.FreedBytes != 1024 {
		t.Fatalf("leftover trash not cleared: exists=%v result=%+v", exists(trash), res)
	}
}

func TestPruneCodexResumeHomesMissingDirIsNoop(t *testing.T) {
	res, err := PruneCodexResumeHomes(t.TempDir(), nil, CodexResumeHomeGrace, time.Now())
	if err != nil || res != (ResumeHomePruneResult{}) {
		t.Fatalf("missing resume-homes: res=%+v err=%v", res, err)
	}
	if res, err := PruneCodexResumeHomes("", nil, CodexResumeHomeGrace, time.Now()); err != nil || res.Scanned != 0 {
		t.Fatalf("empty base: res=%+v err=%v", res, err)
	}
}

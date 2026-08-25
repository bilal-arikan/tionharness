package providers

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPrepareShadowHomeEmptyBase(t *testing.T) {
	dir, cleanup, err := prepareShadowHome("")
	if err != nil || dir != "" {
		t.Fatalf("prepareShadowHome(\"\") = %q, %v", dir, err)
	}
	cleanup()
}

func TestPrepareShadowHomeCopiesAuthSnapshot(t *testing.T) {
	base := t.TempDir()
	auth := filepath.Join(base, "auth.json")
	if err := os.WriteFile(auth, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := prepareShadowHome(base)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(dir) != filepath.Join(base, ".shadow") || !strings.HasPrefix(filepath.Base(dir), "turn-") {
		t.Fatalf("shadow dir %q is not under base", dir)
	}
	shadowAuth := filepath.Join(dir, "auth.json")
	got, err := os.ReadFile(shadowAuth)
	if err != nil || string(got) != "old" {
		t.Fatalf("shadow auth.json = %q, %v", got, err)
	}
	info, err := os.Stat(shadowAuth)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("shadow auth.json mode = %v", info.Mode().Perm())
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("shadow dir remains after cleanup: %v", err)
	}
}

func TestPrepareShadowHomeAuthSnapshotIsIndependent(t *testing.T) {
	base := t.TempDir()
	auth := filepath.Join(base, "auth.json")
	if err := os.WriteFile(auth, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, cleanup, err := prepareShadowHome(base)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	shadowAuth := filepath.Join(dir, "auth.json")
	if err := os.WriteFile(shadowAuth, []byte("shadow-new"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(auth)
	if err != nil || string(got) != "old" {
		t.Fatalf("base auth after shadow write = %q, %v", got, err)
	}
	if err := os.WriteFile(auth, []byte("base-new"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(shadowAuth)
	if err != nil || string(got) != "shadow-new" {
		t.Fatalf("shadow auth after base write = %q, %v", got, err)
	}
}

func TestPrepareShadowHomeWithoutAuth(t *testing.T) {
	dir, cleanup, err := prepareShadowHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(filepath.Join(dir, "auth.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("shadow auth.json should not exist: %v", err)
	}
}

func TestPrepareShadowHomeSweepsOldTurns(t *testing.T) {
	base := t.TempDir()
	shadowRoot := filepath.Join(base, ".shadow")
	oldDir := filepath.Join(shadowRoot, "turn-old")
	newDir := filepath.Join(shadowRoot, "turn-new")
	untouchedDir := filepath.Join(shadowRoot, "keep-old")
	untouchedFile := filepath.Join(shadowRoot, "turn-old-file")
	for _, path := range []string{oldDir, newDir, untouchedDir} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(untouchedFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * shadowHomeMaxAge)
	for _, path := range []string{oldDir, untouchedDir, untouchedFile} {
		if err := os.Chtimes(path, oldTime, oldTime); err != nil {
			t.Fatal(err)
		}
	}

	dir, cleanup, err := prepareShadowHome(base)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(oldDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old turn directory remains: %v", err)
	}
	for _, path := range []string{newDir, untouchedDir, untouchedFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %q to remain: %v", path, err)
		}
	}
	if dir == newDir {
		t.Fatalf("new shadow directory unexpectedly reused %q", dir)
	}
}

func TestPrepareShadowHomeKeepsOldActiveTurn(t *testing.T) {
	base := t.TempDir()
	activeDir := filepath.Join(base, ".shadow", "turn-active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	startedAt, ok := processStartTime(os.Getpid())
	if !ok {
		t.Skip("process start time is unavailable")
	}
	if err := writeShadowHomeOwner(activeDir, startedAt); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * shadowHomeMaxAge)
	if err := os.Chtimes(activeDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	_, cleanup, err := prepareShadowHome(base)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(activeDir); err != nil {
		t.Fatalf("old active turn was removed: %v", err)
	}
}

func TestPrepareShadowHomeSweepsReusedOwnerPID(t *testing.T) {
	base := t.TempDir()
	staleDir := filepath.Join(base, ".shadow", "turn-reused-pid")
	if err := os.MkdirAll(staleDir, 0o700); err != nil {
		t.Fatal(err)
	}
	startedAt, ok := processStartTime(os.Getpid())
	if !ok {
		t.Skip("process start time is unavailable")
	}
	if err := writeShadowHomeOwner(staleDir, startedAt.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * shadowHomeMaxAge)
	if err := os.Chtimes(staleDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	_, cleanup, err := prepareShadowHome(base)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if _, err := os.Stat(staleDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old turn with reused owner PID remains: %v", err)
	}
}

func TestPrepareShadowHomeCopiesCacheFiles(t *testing.T) {
	base := t.TempDir()
	want := map[string]string{
		"models_cache.json": `{"models":["test"]}`,
		"installation_id":   "installation-test-id",
	}
	for name, content := range want {
		if err := os.WriteFile(filepath.Join(base, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	dir, cleanup, err := prepareShadowHome(base)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for name, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != content {
			t.Fatalf("shadow %s = %q, %v", name, got, err)
		}
	}
}

func TestPrepareShadowHomeWithoutCacheFiles(t *testing.T) {
	dir, cleanup, err := prepareShadowHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, name := range []string{"models_cache.json", "installation_id"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("shadow %s should not exist: %v", name, err)
		}
	}
}

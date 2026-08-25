package providers

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

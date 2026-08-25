package providers

import (
	"os"
	"path/filepath"
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

func TestPrepareShadowHomeLinksAuthAndCleansUp(t *testing.T) {
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
	if _, err := os.Stat(shadowAuth); err != nil {
		t.Fatalf("shadow auth.json: %v", err)
	}
	if err := os.WriteFile(shadowAuth, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(auth)
	if err != nil || string(got) != "new" {
		t.Fatalf("base auth after shadow write = %q, %v", got, err)
	}
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("shadow dir remains after cleanup: %v", err)
	}
}

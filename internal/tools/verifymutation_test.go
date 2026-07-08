package tools

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyMutationLanded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	content := []byte("hello world")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Landed: on-disk bytes equal what was written.
	if err := verifyMutationLanded(path, content); err != nil {
		t.Fatalf("verify on matching content: %v", err)
	}

	// Clobbered: another writer changed the file between write and verify.
	if err := os.WriteFile(path, []byte("hello world!!!"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	err := verifyMutationLanded(path, content)
	if err == nil || !strings.Contains(err.Error(), "did not land") {
		t.Fatalf("verify on tampered content = %v, want did-not-land error", err)
	}

	// Same length but different bytes must also fail (not just a size check).
	if err := os.WriteFile(path, []byte("hello_world"), 0o644); err != nil {
		t.Fatalf("tamper2: %v", err)
	}
	if err := verifyMutationLanded(path, content); err == nil {
		t.Fatalf("same-length different bytes passed verification")
	}

	// Removed: the file vanished after the "successful" write.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	err = verifyMutationLanded(path, content)
	if err == nil || !strings.Contains(err.Error(), "cannot be read back") {
		t.Fatalf("verify on missing file = %v, want read-back error", err)
	}

	// Large content (>1MB) takes the hash path.
	big := bytes.Repeat([]byte("x"), (1<<20)+10)
	if err := os.WriteFile(path, big, 0o644); err != nil {
		t.Fatalf("write big: %v", err)
	}
	if err := verifyMutationLanded(path, big); err != nil {
		t.Fatalf("verify big matching: %v", err)
	}
	big[0] = 'y' // want differs from disk now
	if err := verifyMutationLanded(path, big); err == nil {
		t.Fatalf("big mismatch passed verification")
	}
}

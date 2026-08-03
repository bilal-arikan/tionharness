package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A fresh directory gets the embedded starter file.
func TestWriteDefaultGitignoreCreates(t *testing.T) {
	dir := t.TempDir()
	if err := writeDefaultGitignore(dir); err != nil {
		t.Fatalf("writeDefaultGitignore: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	// Spot-check the entries whose absence would be the actual damage: a committed
	// secret and a committed dependency tree.
	for _, want := range []string{".env", "node_modules/"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("starter .gitignore is missing %q", want)
		}
	}
}

// An existing .gitignore is the user's file and must survive untouched.
func TestWriteDefaultGitignoreKeepsExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	const existing = "# mine\nsecret-notes/\n"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := writeDefaultGitignore(dir); err != nil {
		t.Fatalf("writeDefaultGitignore: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != existing {
		t.Errorf("existing .gitignore was overwritten:\n%s", data)
	}
}

package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenQuarantinesCorruptSettings: a hand-edited settings.json with a syntax
// error must not make the application unstartable. Open moves it aside, keeps the
// original bytes recoverable, and continues with defaults.
func TestOpenQuarantinesCorruptSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, fileName)
	broken := []byte(`{"theme": "dark",,, "enableShell": true`)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("write broken settings: %v", err)
	}

	store, err := Open(dir, noopCipher{})
	if err != nil {
		t.Fatalf("a corrupt settings.json must not fail Open: %v", err)
	}
	if got := store.Get(); got.Theme != Default().Theme || got.EnableShell {
		t.Fatalf("store did not fall back to defaults: %+v", got)
	}

	// The rewritten document is valid and holds the defaults.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rewritten settings: %v", err)
	}
	var fresh Settings
	if err := json.Unmarshal(data, &fresh); err != nil {
		t.Fatalf("rewritten settings.json does not parse: %v", err)
	}

	// The user's original file is still recoverable next to it.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	quarantined := ""
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), fileName+".corrupt-") {
			quarantined = filepath.Join(dir, e.Name())
		}
	}
	if quarantined == "" {
		t.Fatalf("no %s.corrupt-<ts> file was written, entries=%v", fileName, entries)
	}
	kept, err := os.ReadFile(quarantined)
	if err != nil {
		t.Fatalf("read quarantined file: %v", err)
	}
	if string(kept) != string(broken) {
		t.Fatalf("quarantined file was modified: %s", kept)
	}
}

// TestOpenCorruptSettingsSurvivesReopen: the second start sees a healthy file and
// must not quarantine anything again.
func TestOpenCorruptSettingsSurvivesReopen(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(`not json at all`), 0o600); err != nil {
		t.Fatalf("write broken settings: %v", err)
	}
	if _, err := Open(dir, noopCipher{}); err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := Open(dir, noopCipher{}); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), fileName+".corrupt-") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("quarantine count = %d, want exactly 1", n)
	}
}

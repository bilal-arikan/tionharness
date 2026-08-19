package providers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClaudeCLICanResume covers the pre-flight that keeps a moved config home
// from turning every stored resume id into three dead turns.
func TestClaudeCLICanResume(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "C--Users-x-repo")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	live := "0faeefbe-6af0-46dd-818c-af74a644c7bc"
	if err := os.WriteFile(filepath.Join(projectDir, live+".jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &ClaudeCLI{configDir: home}
	if !c.CanResume(live) {
		t.Errorf("transcript exists under projects/: expected resumable")
	}
	if c.CanResume("11111111-2222-3333-4444-555555555555") {
		t.Errorf("no transcript for this id: expected NOT resumable")
	}
	if c.CanResume("") {
		t.Errorf("empty id is never resumable")
	}

	// No config dir → the CLI uses the ambient home, which is not ours to inspect;
	// never downgrade a healthy warm session on a guess.
	if !(&ClaudeCLI{}).CanResume(live) {
		t.Errorf("unknown config home: expected resumable (no authoritative answer)")
	}
	// A home with no projects/ directory at all is equally unknowable.
	if !(&ClaudeCLI{configDir: t.TempDir()}).CanResume(live) {
		t.Errorf("missing projects dir: expected resumable (no authoritative answer)")
	}
}

func TestIsMissingConversation(t *testing.T) {
	stale := "No conversation found with session ID: 0faeefbe-6af0-46dd-818c-af74a644c7bc"
	if !isMissingConversation(stale) {
		t.Errorf("expected the stale-resume rejection to be recognised")
	}
	if isMissingConversation("Not logged in · Please run /login") {
		t.Errorf("an auth failure is not a stale-resume rejection")
	}
}

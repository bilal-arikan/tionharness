package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWorkspaceCodexHomeDir locks the codex-home path shape against its
// claude-home sibling: same "workDir's parent + <name>-home" pattern, empty
// workDir stays empty (no pinning, global CODEX_HOME default).
func TestWorkspaceCodexHomeDir(t *testing.T) {
	cases := []struct {
		workDir string
		want    string
	}{
		{"", ""},
		{filepath.Join("C:", "ws", "workspace"), filepath.Join("C:", "ws", "codex-home")},
	}
	for _, c := range cases {
		if got := workspaceCodexHomeDir(c.workDir); got != c.want {
			t.Errorf("workspaceCodexHomeDir(%q) = %q, want %q", c.workDir, got, c.want)
		}
	}
}

// TestEnsureWorkspaceCodexHome_SeedsFromGlobal covers the main case: a fresh
// workspace with no codex-home yet, and a global ~/.codex that has a login.
func TestEnsureWorkspaceCodexHome_SeedsFromGlobal(t *testing.T) {
	globalHome := t.TempDir()
	t.Setenv("CODEX_HOME", globalHome)
	if err := os.WriteFile(filepath.Join(globalHome, "auth.json"), []byte(`{"tokens":"secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// config.toml must NOT be copied — TionSwarm renders its own per turn.
	if err := os.WriteFile(filepath.Join(globalHome, "config.toml"), []byte(`model = "gpt-5"`), 0o644); err != nil {
		t.Fatal(err)
	}

	wsRoot := t.TempDir()
	// globalCodexHomeDir must resolve the SEED source, not the workspace's own
	// CODEX_HOME — so point the env at the global home, distinct from wsRoot.
	EnsureWorkspaceCodexHome(wsRoot)

	dst := filepath.Join(wsRoot, "codex-home", "auth.json")
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("expected seeded auth.json, got error: %v", err)
	}
	if string(got) != `{"tokens":"secret"}` {
		t.Errorf("auth.json content = %q, want seeded content", got)
	}

	if _, err := os.Stat(filepath.Join(wsRoot, "codex-home", "config.toml")); !os.IsNotExist(err) {
		t.Errorf("config.toml should NOT be copied, stat err = %v", err)
	}
}

// TestEnsureWorkspaceCodexHome_NoopWhenWorkspaceAuthExists asserts the
// workspace's own login is never clobbered by a heal.
func TestEnsureWorkspaceCodexHome_NoopWhenWorkspaceAuthExists(t *testing.T) {
	globalHome := t.TempDir()
	t.Setenv("CODEX_HOME", globalHome)
	if err := os.WriteFile(filepath.Join(globalHome, "auth.json"), []byte(`{"tokens":"global"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	wsRoot := t.TempDir()
	home := filepath.Join(wsRoot, "codex-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(home, "auth.json")
	if err := os.WriteFile(existing, []byte(`{"tokens":"workspace-own"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	EnsureWorkspaceCodexHome(wsRoot)

	got, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"tokens":"workspace-own"}` {
		t.Errorf("existing workspace auth.json was modified: got %q", got)
	}
}

// TestEnsureWorkspaceCodexHome_NoopWhenNoGlobalAuth asserts a missing global
// login is a safe no-op (no panic, no partial codex-home left in a confusing
// half-state), since this is the exact case the user hit.
func TestEnsureWorkspaceCodexHome_NoopWhenNoGlobalAuth(t *testing.T) {
	globalHome := t.TempDir() // exists but has no auth.json
	t.Setenv("CODEX_HOME", globalHome)

	wsRoot := t.TempDir()
	EnsureWorkspaceCodexHome(wsRoot)

	if _, err := os.Stat(filepath.Join(wsRoot, "codex-home", "auth.json")); !os.IsNotExist(err) {
		t.Errorf("expected no auth.json to be created, stat err = %v", err)
	}
}

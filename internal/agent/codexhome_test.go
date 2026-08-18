package agent

import (
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

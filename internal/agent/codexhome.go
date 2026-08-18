package agent

import "path/filepath"

// workspaceCodexHomeDir is a workspace's per-workspace codex-cli config home
// (<workspace>/codex-home), a sibling of store/, config/ and workspace/. It is
// exported into the CLI subprocess as CODEX_HOME so every workspace runs the CLI
// against its OWN auth.json/config.toml instead of one global home — the exact
// analogue of workspaceClaudeHomeDir. workDir is the sandbox root
// (<workspace>/workspace); the home is its sibling. Empty when workDir is unknown.
func workspaceCodexHomeDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "codex-home")
}

// codexHomeDir returns this runtime's workspace codex-cli config home.
func (r *Runtime) codexHomeDir() string { return workspaceCodexHomeDir(r.workDir) }

// CodexHomeDir exposes this workspace's codex-cli config home for out-of-loop
// call sites that need it outside the per-turn seam.
func (r *Runtime) CodexHomeDir() string { return r.codexHomeDir() }

package agent

import "path/filepath"

// appCodexHomeDir resolves the shared app-global codex-cli config home.
func appCodexHomeDir(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	return filepath.Join(dataDir, "codex-home")
}

// codexHomeDir returns this runtime's app-global codex-cli config home.
func (r *Runtime) codexHomeDir() string { return appCodexHomeDir(r.dataDir) }

// CodexHomeDir exposes the app-global codex-cli config home for out-of-loop
// call sites that need it outside the per-turn seam.
func (r *Runtime) CodexHomeDir() string { return r.codexHomeDir() }

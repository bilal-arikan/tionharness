package agent

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveCLIHomeDir returns an instance's explicit config home, or the shared
// app-global home for a built-in CLI kind. dataDir is required for the fallback
// so callers cannot accidentally derive authentication state from a workspace.
func ResolveCLIHomeDir(dataDir, kindID, configDir string) (string, error) {
	if home := strings.TrimSpace(configDir); home != "" {
		return home, nil
	}
	if strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("resolve app-global %s home: data dir is empty", kindID)
	}
	home := appCLIHomeDir(dataDir, kindID)
	if home == "" {
		return "", fmt.Errorf("resolve CLI home: unsupported provider kind %q", kindID)
	}
	return home, nil
}

func appCLIHomeDir(dataDir, kindID string) string {
	switch kindID {
	case "claude-cli":
		return filepath.Join(dataDir, "claude-home")
	case "codex-cli":
		return filepath.Join(dataDir, "codex-home")
	default:
		return ""
	}
}

// appCodexHomeDir resolves the shared app-global codex-cli config home.
func appCodexHomeDir(dataDir string) string {
	return appCLIHomeDir(dataDir, "codex-cli")
}

// codexHomeDir returns this runtime's app-global codex-cli config home.
func (r *Runtime) codexHomeDir() string { return appCodexHomeDir(r.dataDir) }

// CodexHomeDir exposes the app-global codex-cli config home for out-of-loop
// call sites that need it outside the per-turn seam.
func (r *Runtime) CodexHomeDir() string { return r.codexHomeDir() }

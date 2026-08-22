package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bilal-arikan/tionswarm/internal/providers"
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

// PinCLIHome is the ONE entry point every provider.Complete site must call
// before handing a request to a CLI transport: it pins the claude-cli home AND
// the codex-cli home, each a no-op for the other transport. Sites that pinned
// only claude (manual /compact, /handoff, the streaming pre-turn compaction)
// left CODEX_HOME unexported, so a codex-cli agent silently ran against the
// ambient ~/.codex — a different, frequently revoked login than the one
// TionSwarm created. Call this instead of the two Pin*Home helpers.
func (r *Runtime) PinCLIHome(provider providers.Provider) error {
	if _, err := r.PinClaudeHome(provider); err != nil {
		return err
	}
	return r.PinCodexHome(provider)
}

// PinCodexHome is the codex-cli sibling of PinClaudeHome and the single place
// the CODEX_HOME of a turn is decided. An instance that carries its own
// configDir already owns a home and keeps it (the ConfigDir()=="" guard);
// otherwise the app-global <dataDir>/codex-home is pinned, so the subprocess
// reads the login TionSwarm actually created instead of the ambient ~/.codex —
// which is frequently a different, stale or revoked credential.
//
// The MkdirAll is load-bearing, not defensive: CODEX_HOME must point at an
// EXISTING directory or the subprocess errors out immediately (unlike
// claude-cli, which tolerates a missing home), and there is no boot-time
// provisioning step for codex-home yet. It runs for an instance's OWN configDir
// too, so a missing login surfaces as codex's own clear auth error rather than a
// confusing "CODEX_HOME is not an existing directory" one.
//
// No credential heal here: codex's auth.json is not known to be wiped by a
// losing refresh race the way claude's credentials.json is, so mirroring
// ensureClaudeHomeCredential would be speculative until observed.
//
// No-op (nil) for non-codex-cli providers. Errors are returned, never swallowed:
// a turn that cannot resolve or create its home would otherwise silently fall
// back to the ambient home and fail with an unrelated-looking auth error.
func (r *Runtime) PinCodexHome(provider providers.Provider) error {
	cx, ok := provider.(*providers.CodexCLI)
	if !ok {
		return nil
	}
	home := cx.ConfigDir()
	if home == "" {
		resolved, err := ResolveCLIHomeDir(r.dataDir, "codex-cli", "")
		if err != nil {
			return err
		}
		home = resolved
		cx.SetConfigDir(home)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		return fmt.Errorf("create codex home %s: %w", home, err)
	}
	return nil
}

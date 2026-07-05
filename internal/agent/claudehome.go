package agent

import (
	"io"
	"os"
	"path/filepath"
)

// workspaceClaudeHomeDir is a workspace's per-workspace claude-cli config home
// (<workspace>/claude-home), a sibling of store/, config/ and workspace/. It is
// exported into the CLI subprocess as CLAUDE_CONFIG_DIR so every workspace runs the
// CLI against its OWN skills/settings/login instead of one global home. workDir is
// the sandbox root (<workspace>/workspace); the home is its sibling. Empty when
// workDir is unknown.
func workspaceClaudeHomeDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(workDir), "claude-home")
}

// globalClaudeHomeDir mirrors settings.defaultClaudeConfigDir: the TionSwarm-managed
// global claude-cli config home (~/.tionswarm/claude-home) that holds the shared
// login/settings before per-workspace homes existed. It is the seed source copied
// into a fresh per-workspace home so the workspace CLI starts already authenticated.
// Empty when the user home cannot be resolved (no seed → the CLI relies on the
// injected auth env instead).
func globalClaudeHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".tionswarm", "claude-home")
}

// claudeHomeEphemeralDirs are per-run/history subdirectories NOT copied when seeding
// a per-workspace claude-home from the global one: they are large and run-specific
// (session transcripts, shell snapshots), not config the workspace should inherit.
var claudeHomeEphemeralDirs = map[string]bool{
	"projects":        true, // per-project session transcripts
	"sessions":        true, // CLI session storage
	"session-env":     true,
	"file-history":    true, // edit history snapshots
	"shell-snapshots": true,
	"todos":           true,
	"tasks":           true, // per-run scheduled tasks (avoid duplicating across workspaces)
	"cache":           true, // large, regenerable
	"backups":         true,
	"statsig":         true,
	"context-mode":    true,
	"logs":            true,
}

// EnsureWorkspaceClaudeHome provisions the per-workspace claude-cli config home for a
// workspace root (<workspace>). It is idempotent and safe to call on every workspace
// open: if <workspace>/claude-home does not exist, it creates it and seeds it from the
// global ~/.tionswarm/claude-home (config + login, skipping per-run/history dirs) so
// the workspace CLI starts already logged in.
//
// Skills are NOT stored here: the workspace skill tier stays at <workspace>/skills
// (served by TionSwarm's use_skill bridge; the CLI's native Skill tool is disabled —
// see climcp.go). claude-home holds only the CLI's login/settings.
//
// Filesystem errors are best-effort: a failed seed leaves the workspace usable (auth
// still flows via the injected env) rather than blocking workspace open.
func EnsureWorkspaceClaudeHome(wsRoot string) {
	if wsRoot == "" {
		return
	}
	home := filepath.Join(wsRoot, "claude-home")

	if _, err := os.Stat(home); os.IsNotExist(err) {
		_ = os.MkdirAll(home, 0o755)
		if src := globalClaudeHomeDir(); src != "" && src != home {
			if fi, statErr := os.Stat(src); statErr == nil && fi.IsDir() {
				_ = copyClaudeHome(src, home)
			}
		}
	}
}

// copyClaudeHome recursively copies the config-relevant contents of the global
// claude-home into a fresh per-workspace home, skipping the ephemeral per-run/history
// dirs (claudeHomeEphemeralDirs). Only top-level ephemeral dirs are skipped; nested
// content is copied verbatim.
func copyClaudeHome(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() && claudeHomeEphemeralDirs[name] {
			continue
		}
		if err := copyPath(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return err
		}
	}
	return nil
}

// copyPath copies a single file or directory tree from src to dst, preserving the
// source's file mode. Symlinks are followed as regular files.
func copyPath(src, dst string) error {
	fi, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst, fi.Mode())
}

// copyFile copies a regular file's contents from src to dst with the given mode.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

package agent

import (
	"log"
	"os"
	"path/filepath"
)

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

// globalCodexHomeDir resolves the codex CLI's OWN global config home the same
// way the codex binary itself does: the CODEX_HOME env var if set, else
// ~/.codex. This is deliberately NOT ~/.tionswarm/codex-home — unlike claude,
// TionSwarm has no shared managed codex home; the only pre-existing login a
// fresh workspace can inherit is whatever the user already logged into
// directly with `codex login`.
func globalCodexHomeDir() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// EnsureWorkspaceCodexHome provisions the per-workspace codex-cli config home
// for a workspace root (<workspace>). It is idempotent and safe to call on
// every workspace open: if <workspace>/codex-home/auth.json does not already
// exist, it is seeded from the global codex home's auth.json (see
// globalCodexHomeDir) so the workspace CLI starts already logged in instead of
// requiring a separate `codex login` per workspace.
//
// Unlike EnsureWorkspaceClaudeHome, this does NOT copy config.toml: TionSwarm
// renders that file itself per turn (writeCodexConfig in
// internal/providers/codexcli_config.go), so seeding a user's global one would
// at best be immediately overwritten and at worst leave stale/conflicting
// settings lying around. There is also no credential-liveness heal here —
// unlike the claude CLI, codex has no known behavior of wiping its own
// auth.json on a failed refresh, so there is nothing to self-heal against;
// seeding an absent file is the whole job.
//
// Filesystem errors are best-effort: a failed seed leaves the workspace usable
// (the user can still `codex login` directly against the workspace's
// CODEX_HOME) rather than blocking workspace open. Skips and failures are
// logged, though, because a silently-unseeded workspace is exactly the
// confusing "not logged in" surprise this function exists to prevent.
func EnsureWorkspaceCodexHome(wsRoot string) {
	if wsRoot == "" {
		return
	}
	home := filepath.Join(wsRoot, "codex-home")
	dst := filepath.Join(home, "auth.json")

	if _, err := os.Stat(dst); err == nil {
		return // already seeded/logged in for this workspace — never clobber
	}

	src := globalCodexHomeDir()
	if src == "" {
		log.Printf("codex-home: skipping seed for %s: cannot resolve global codex home (no HOME)", home)
		return
	}
	srcAuth := filepath.Join(src, "auth.json")
	if _, err := os.Stat(srcAuth); err != nil {
		log.Printf("codex-home: skipping seed for %s: no global auth.json at %s (not logged in via `codex login`?)", home, srcAuth)
		return
	}

	if err := os.MkdirAll(home, 0o755); err != nil {
		log.Printf("codex-home: failed to create %s: %v", home, err)
		return
	}
	if err := copyFile(srcAuth, dst, 0o600); err != nil {
		log.Printf("codex-home: failed to seed %s from %s: %v", dst, srcAuth, err)
		return
	}
	log.Printf("codex-home: seeded %s from global codex home %s", dst, src)
}

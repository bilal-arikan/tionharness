package agent

import (
	"context"
	"os"
	"os/exec"

	"github.com/bilal/swarmgo/internal/proc"
	"path/filepath"
	"strings"
)

// gitRepoToplevel returns the repository root for dir, or "" when dir is not in a
// git working tree (or git is unavailable). Used to decide whether a working dir
// can be worktree-isolated and to scope worktree operations to the right repo.
func gitRepoToplevel(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--show-toplevel")
	proc.Hide(cmd) // no console flash under the windowless desktop app
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// worktreesDir is where per-session worktrees live: <workspace>/worktrees,
// a sibling of store/, config/ and workspace/. Empty when workDir is unknown.
func (r *Runtime) worktreesDir() string {
	if r.workDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(r.workDir), "worktrees")
}

// ensureWorktree returns a per-session git worktree for the repository that
// contains base, creating it on first use. It is the step-4 isolation brake: when
// enabled, autonomous sessions edit an isolated worktree (+ branch) instead of
// the shared working tree, so parallel agents never clobber each other's files.
//
// Best-effort: when base is not a git repo, git is missing, or worktree creation
// fails, it falls back to base (logged) so a turn never breaks. Idempotent — an
// existing worktree dir is reused.
func (r *Runtime) ensureWorktree(ctx context.Context, base, sessionID string) string {
	if sessionID == "" || r.worktreesDir() == "" {
		return base
	}
	repo := gitRepoToplevel(ctx, base)
	if repo == "" {
		return base // not a git repo — nothing to isolate
	}
	wtPath := filepath.Join(r.worktreesDir(), sessionID)
	if info, err := os.Stat(wtPath); err == nil && info.IsDir() {
		return wtPath // already created for this session
	}
	if err := os.MkdirAll(r.worktreesDir(), 0o755); err != nil {
		r.logger.Warn("worktree mkdir failed", "error", err)
		return base
	}
	branch := "swarmgo/session-" + sessionID
	// -b creates the branch at HEAD; if it already exists (re-create after a manual
	// rmdir) fall back to attaching without -b.
	addCmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", "-b", branch, wtPath, "HEAD")
	proc.Hide(addCmd)
	if out, err := addCmd.CombinedOutput(); err != nil {
		retryCmd := exec.CommandContext(ctx, "git", "-C", repo, "worktree", "add", wtPath)
		proc.Hide(retryCmd)
		if out2, err2 := retryCmd.CombinedOutput(); err2 != nil {
			r.logger.Warn("worktree add failed", "error", err, "out", string(out), "retryErr", err2, "retryOut", string(out2))
			return base
		}
	}
	r.logger.Info("created session worktree", "session", sessionID, "path", wtPath, "branch", branch)
	return wtPath
}

// RemoveSessionWorktree tears down a session's worktree, if any (best-effort).
// Safe to call unconditionally on session delete: a no-op when the session never
// had a worktree. Exposed for the API delete handler.
func (r *Runtime) RemoveSessionWorktree(sessionID string) {
	if sessionID == "" || r.worktreesDir() == "" {
		return
	}
	wtPath := filepath.Join(r.worktreesDir(), sessionID)
	if _, err := os.Stat(wtPath); err != nil {
		return // nothing to remove
	}
	// `git worktree remove` cleans up the repo's worktree registry; --force allows
	// removal even with uncommitted changes. Fall back to a plain rmdir so the dir
	// is never leaked when git is unavailable.
	rmCmd := exec.Command("git", "-C", wtPath, "worktree", "remove", "--force", wtPath)
	proc.Hide(rmCmd)
	if out, err := rmCmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(wtPath)
		r.logger.Warn("worktree remove fell back to rmdir", "session", sessionID, "error", err, "out", string(out))
	}
}

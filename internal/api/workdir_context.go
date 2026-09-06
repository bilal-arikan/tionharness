package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

// gitBranchCtxTimeout bounds the `git` calls used for the working-dir context
// block so a slow/hung git can never stall a turn.
const gitBranchCtxTimeout = 3 * time.Second

// gitBranch returns the current branch name for dir, or "" when dir is not a git
// repo (or git is unavailable). Best-effort and time-bounded.
func gitBranch(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitBranchCtxTimeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	// Same reason as runGit: a git helper that survives the timeout would keep the
	// output pipe open and wedge this call.
	proc.TreeKill(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// workdirContextBlock renders the agent's working directory (cwd) as a
// system-prompt section, mirroring the external agent project: it tells the agent where its
// file/shell tools operate, the git branch when the dir is a repo, and whether a
// CLAUDE.md is present (so it knows to read project conventions). Returns "" when
// dir is empty. Kept in the dynamic (uncached) suffix because the branch can
// change mid-session.
func workdirContextBlock(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## Working directory\n")
	b.WriteString("Your file and shell tools operate from this directory. Relative paths resolve here; you may also use absolute paths.\n\n")
	b.WriteString("- Path: `" + dir + "`\n")
	if br := gitBranch(dir); br != "" {
		b.WriteString("- Git branch: `" + br + "`\n")
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		b.WriteString("- A `CLAUDE.md` is present — read it for project structure, conventions and build/test commands.\n")
	}
	return strings.TrimSpace(b.String())
}

// Package worktree owns git worktree operations for card lifecycles.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

const gitErrorOutputLimit = 8 * 1024

var (
	ErrConflict = errors.New("git merge conflict")
	ErrDirty    = errors.New("worktree has uncommitted or unmerged work")
)

// Runner makes command execution replaceable in unit tests.
type Runner interface {
	Run(ctx context.Context, dir, name string, args ...string) (string, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := proc.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type Git struct {
	RepoRoot string
	Runner   Runner
}

func (g Git) run(ctx context.Context, args ...string) (string, error) {
	if g.Runner == nil {
		return "", errors.New("worktree git runner is nil")
	}
	out, err := g.Runner.Run(ctx, g.RepoRoot, "git", append([]string{"-C", g.RepoRoot}, args...)...)
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, truncateGitErrorOutput(out))
	}
	return out, nil
}

func truncateGitErrorOutput(out string) string {
	out = strings.TrimSpace(out)
	if len(out) <= gitErrorOutputLimit {
		return out
	}
	lastLineStart := strings.LastIndexByte(out, '\n') + 1
	tail := ""
	if lastLineStart > 0 && len(out)-lastLineStart < gitErrorOutputLimit/4 {
		tail = strings.TrimSpace(out[lastLineStart:])
	}
	marker := fmt.Sprintf("\n… [%d bytes omitted]", len(out))
	headBudget := gitErrorOutputLimit - len(marker)
	if tail != "" {
		headBudget -= len(tail) + 1
	}
	head := validUTF8Prefix(out, headBudget)
	omitted := len(out) - len(head) - len(tail)
	marker = fmt.Sprintf("\n… [%d bytes omitted]", omitted)
	headBudget = gitErrorOutputLimit - len(marker)
	if tail != "" {
		headBudget -= len(tail) + 1
	}
	head = validUTF8Prefix(out, headBudget)
	omitted = len(out) - len(head) - len(tail)
	marker = fmt.Sprintf("\n… [%d bytes omitted]", omitted)
	if tail != "" {
		return head + marker + "\n" + tail
	}
	return head + marker
}

func validUTF8Prefix(s string, limit int) string {
	if limit >= len(s) {
		return s
	}
	if limit <= 0 {
		return ""
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit]
}

// ResolveBaseRef validates an explicit ref or resolves the repository's current
// branch. It never guesses a conventional branch name.
func (g Git) ResolveBaseRef(ctx context.Context, configured string) (string, error) {
	if configured != "" {
		if _, err := g.run(ctx, "rev-parse", "--verify", configured+"^{commit}"); err != nil {
			return "", fmt.Errorf("invalid worktree base ref %q: %w", configured, err)
		}
		return configured, nil
	}
	resolved, err := g.run(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		return "", fmt.Errorf("cannot resolve worktree base ref: HEAD is detached: %w", err)
	}
	ref := strings.TrimSpace(resolved)
	if ref == "" {
		return "", errors.New("cannot resolve worktree base ref: HEAD has no branch")
	}
	if _, err := g.run(ctx, "rev-parse", "--verify", ref+"^{commit}"); err != nil {
		return "", fmt.Errorf("cannot resolve worktree base ref: HEAD branch %q is unborn: %w", ref, err)
	}
	return ref, nil
}

func (g Git) Provision(ctx context.Context, branch, path, baseRef string) error {
	_, err := g.run(ctx, "worktree", "add", "-b", branch, path, baseRef)
	return err
}

// Merge merges branch into an already checked-out base branch, then removes
// the integrated worktree and branch. A conflict is typed and left intact.
func (g Git) Merge(ctx context.Context, branch, path, baseRef string) error {
	head, err := g.run(ctx, "branch", "--show-current")
	if err != nil {
		return err
	}
	if strings.TrimSpace(head) != baseRef {
		return fmt.Errorf("base checkout is %q, want %q", strings.TrimSpace(head), baseRef)
	}
	status, err := g.run(ctx, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w: base worktree has staged, unstaged, or untracked changes", ErrDirty)
	}
	if _, err := g.run(ctx, "merge", "--no-ff", branch); err != nil {
		unmerged, inspectErr := g.run(ctx, "diff", "--name-only", "--diff-filter=U")
		if inspectErr == nil && strings.TrimSpace(unmerged) != "" {
			return fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return err
	}
	if _, err := g.run(ctx, "worktree", "remove", path); err != nil {
		return err
	}
	_, err = g.run(ctx, "branch", "-d", branch)
	return err
}

// Discard refuses to remove a worktree containing either uncommitted changes
// or commits not reachable from baseRef.
func (g Git) Discard(ctx context.Context, branch, path, baseRef string) error {
	status, err := g.run(ctx, "-C", path, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return fmt.Errorf("%w: uncommitted changes", ErrDirty)
	}
	countText, err := g.run(ctx, "rev-list", "--count", baseRef+".."+branch)
	if err != nil {
		return err
	}
	count, err := strconv.Atoi(strings.TrimSpace(countText))
	if err != nil {
		return fmt.Errorf("parse unmerged commit count %q: %w", strings.TrimSpace(countText), err)
	}
	if count > 0 {
		return fmt.Errorf("%w: %d commit(s) not merged into %s", ErrDirty, count, baseRef)
	}
	if _, err := g.run(ctx, "worktree", "remove", path); err != nil {
		return err
	}
	_, err = g.run(ctx, "branch", "-d", branch)
	return err
}

// Package worktree owns git worktree operations for card lifecycles.
package worktree

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/proc"
)

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
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(out))
	}
	return out, nil
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
	if _, err := g.run(ctx, "diff", "--quiet"); err != nil {
		return fmt.Errorf("base worktree is dirty: %w", err)
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

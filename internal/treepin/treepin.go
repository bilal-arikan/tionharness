// Package treepin pins the working-tree state a validator verifies to the files
// in a card's scope, so a verdict goes STALE only when THOSE files change —
// not when an unrelated file elsewhere in a shared working tree does.
//
// A pin records HEAD and one digest over the current content of every file
// matched by the scope pathspecs (tracked or untracked, ignoring .gitignore'd
// files). Verify recomputes the digest: equal means the validated tree is still
// the tree on disk for everything in scope, whatever happened outside it.
package treepin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Pin is a captured, scoped tree state.
type Pin struct {
	Head   string   // commit HEAD pointed at when the pin was taken
	Scope  []string // git pathspecs, repository-relative
	Digest string   // hex sha256 over (path, content) of every in-scope file
	Files  int      // how many files the digest covers
}

// Result is the outcome of verifying a pin against the current tree.
type Result struct {
	Stale bool
	// DirtyInScope lists in-scope files that currently differ from HEAD; on a
	// STALE result they are the first candidates for what moved.
	DirtyInScope []string
	// HeadMoved and DirtyOutOfScope describe churn OUTSIDE the scope. They are
	// reported for context and never make the result stale.
	HeadMoved       bool
	CurrentHead     string
	DirtyOutOfScope int
}

// Capture pins the scoped state of the repository containing dir.
func Capture(dir string, scope []string) (Pin, error) {
	scope = cleanScope(scope)
	if len(scope) == 0 {
		return Pin{}, errors.New("treepin: a pin needs at least one scope path (the card's files); a whole-tree pin is what goes stale on unrelated churn")
	}
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Pin{}, err
	}
	head, err := git(top, "rev-parse", "HEAD")
	if err != nil {
		return Pin{}, err
	}
	digest, n, err := scopeDigest(top, scope)
	if err != nil {
		return Pin{}, err
	}
	return Pin{Head: head, Scope: scope, Digest: digest, Files: n}, nil
}

// Verify compares p against the current tree of the repository containing dir.
func Verify(dir string, p Pin) (Result, error) {
	if len(p.Scope) == 0 {
		return Result{}, errors.New("treepin: pin has no scope")
	}
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Result{}, err
	}
	head, err := git(top, "rev-parse", "HEAD")
	if err != nil {
		return Result{}, err
	}
	digest, _, err := scopeDigest(top, p.Scope)
	if err != nil {
		return Result{}, err
	}
	inScope, outScope, err := dirtySplit(top, p.Scope)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Stale:           digest != p.Digest,
		DirtyInScope:    inScope,
		HeadMoved:       head != p.Head,
		CurrentHead:     head,
		DirtyOutOfScope: outScope,
	}, nil
}

func cleanScope(scope []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range scope {
		s = filepath.ToSlash(strings.TrimSpace(s))
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// scopeDigest hashes every file the scope matches under the repository root
// top. A tracked file deleted from disk contributes a deletion marker, so
// deleting an in-scope file is a change.
func scopeDigest(top string, scope []string) (string, int, error) {
	args := append([]string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--"}, scope...)
	out, err := gitRaw(top, args...)
	if err != nil {
		return "", 0, err
	}
	paths := splitZ(out)
	sort.Strings(paths)
	h := sha256.New()
	prev := ""
	n := 0
	for _, p := range paths {
		if p == prev { // ls-files repeats a path that is both cached and unmerged
			continue
		}
		prev = p
		n++
		fmt.Fprintf(h, "%s\x00", p)
		if err := hashFile(h, filepath.Join(top, filepath.FromSlash(p))); err != nil {
			return "", 0, err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

func hashFile(h io.Writer, path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		_, _ = io.WriteString(h, "<deleted>\x00")
		return nil
	}
	if err != nil {
		return fmt.Errorf("treepin: read %s: %w", path, err)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("treepin: stat %s: %w", path, err)
	}
	if fi.IsDir() { // a submodule or nested repository: its name is all we pin
		_, _ = io.WriteString(h, "<dir>\x00")
		return nil
	}
	fmt.Fprintf(h, "%d\x00", fi.Size())
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("treepin: read %s: %w", path, err)
	}
	return nil
}

// dirtySplit partitions `git status` entries (run from the repository root, so
// scope pathspecs stay repository-relative) into in-scope paths and a count of
// out-of-scope ones.
func dirtySplit(dir string, scope []string) ([]string, int, error) {
	all, err := dirtyPaths(dir, nil)
	if err != nil {
		return nil, 0, err
	}
	in, err := dirtyPaths(dir, scope)
	if err != nil {
		return nil, 0, err
	}
	return in, len(all) - len(in), nil
}

func dirtyPaths(dir string, scope []string) ([]string, error) {
	args := []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}
	if len(scope) > 0 {
		args = append(append(args, "--"), scope...)
	}
	out, err := gitRaw(dir, args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	entries := splitZ(out)
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		paths = append(paths, e[3:])
		if e[0] == 'R' || e[0] == 'C' {
			i++ // the rename/copy source follows as its own NUL-terminated entry
		}
	}
	return paths, nil
}

func splitZ(s string) []string {
	var out []string
	for _, p := range strings.Split(s, "\x00") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

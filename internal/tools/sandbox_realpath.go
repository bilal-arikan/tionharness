package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// realPathOf resolves abs to the path the OS actually reaches when it opens abs,
// following every symlink and (on Windows) junction on the way. The deepest
// EXISTING ancestor is resolved by the platform — through an opened handle on
// Windows (see finalPath) — and the not-yet-existing remainder is appended
// lexically: a component that does not exist cannot be a link.
//
// A dangling link (the entry exists but its target does not) is an error rather
// than a "not exist" walk-up: appending its name lexically would approve a write
// that the OS then performs at the link's target, wherever that is.
func realPathOf(abs string) (string, error) {
	cur := abs
	var tail []string
	for {
		real, err := finalPath(cur)
		if err == nil {
			for _, t := range slices.Backward(tail) {
				real = filepath.Join(real, t)
			}
			return filepath.Clean(real), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("resolve real path of %q: %w", cur, err)
		}
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", fmt.Errorf("path %q is a link whose target does not exist", cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			// Not even the volume root exists; there is nothing a link could redirect.
			return filepath.Clean(abs), nil
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}

// checkRealPathUnderRoot is the link-aware half of the confined boundary: after
// the lexical check has passed, it resolves both Root and abs to their real
// locations and rejects abs when a symlink or junction INSIDE Root redirects it
// outside. Root is resolved on every call (not cached) because it may be created
// or re-pointed after the sandbox was built.
//
// This narrows but does not eliminate the check-then-open window: a link created
// between Resolve and the caller's open is not seen. Closing that fully needs
// open-by-handle plumbing through every fs tool.
func checkRealPathUnderRoot(abs, root, rel string) error {
	realRoot, err := realPathOf(root)
	if err != nil {
		return fmt.Errorf("sandbox root: %w", err)
	}
	realAbs, err := realPathOf(abs)
	if err != nil {
		return fmt.Errorf("path %q: %w", rel, err)
	}
	if !underRoot(realAbs, realRoot) {
		return fmt.Errorf("path %q escapes the sandbox through a symlink or junction (real location %q is outside %q)", rel, realAbs, realRoot)
	}
	return nil
}

// realPathUnderRoot reports whether abs's real location lies inside root's real
// location. Any resolution error counts as "not under".
func realPathUnderRoot(abs, root string) bool {
	realRoot, err := realPathOf(root)
	if err != nil {
		return false
	}
	realAbs, err := realPathOf(abs)
	if err != nil {
		return false
	}
	return underRoot(realAbs, realRoot)
}

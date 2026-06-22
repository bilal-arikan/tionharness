package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Sandbox is the base directory for filesystem and shell tools. Root is the
// location relative paths resolve against (the agent's working dir). By default
// it is NOT a confinement boundary: path confinement is intentionally disabled
// for the workspace fs/shell tools, so they may read, write and run anywhere on
// the machine — the permission layer (read-only / ask / auto) is the safety
// boundary instead. Absolute paths are honoured as-is; relative paths resolve
// against Root.
//
// When Confined is true the original strict behaviour is kept: absolute paths
// and ".." escapes are rejected so a tool can never leave Root. This is used for
// the narrow config-dir sandbox, where an agent may only edit its own
// <workspace>/config/ files.
type Sandbox struct {
	Root     string // absolute, cleaned base dir for relative paths (default cwd)
	Confined bool   // when true, reject absolute paths and ".." escapes
}

// NewSandbox builds an UNCONFINED sandbox based at dir (workspace fs/shell). dir
// is made absolute and cleaned; an empty dir yields a sandbox that resolves
// relative paths against the process working directory.
func NewSandbox(dir string) Sandbox {
	return Sandbox{Root: cleanRoot(dir)}
}

// NewConfinedSandbox builds a sandbox that confines every resolved path to dir
// (rejecting absolute inputs and ".." escapes). An empty dir yields a sandbox
// whose Resolve always fails.
func NewConfinedSandbox(dir string) Sandbox {
	return Sandbox{Root: cleanRoot(dir), Confined: true}
}

// cleanRoot makes dir absolute and cleaned; empty/erroring input yields "".
func cleanRoot(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}

// Ready reports whether the sandbox has a configured base directory.
func (s Sandbox) Ready() bool { return s.Root != "" }

// Resolve turns an agent-supplied path into an absolute path.
//
// Unconfined (default): absolute inputs are honoured as-is; relative inputs
// resolve against Root (or, when Root is empty, the process working directory);
// ".." escapes are allowed.
//
// Confined: requires a configured Root, rejects absolute inputs, and rejects any
// path that would escape Root via "..". The empty path resolves to Root.
func (s Sandbox) Resolve(rel string) (string, error) {
	rel = strings.TrimSpace(rel)

	if s.Confined {
		if !s.Ready() {
			return "", fmt.Errorf("filesystem sandbox is not configured")
		}
		if filepath.IsAbs(rel) {
			return "", fmt.Errorf("absolute paths are not allowed; use a path relative to the root")
		}
		abs := filepath.Clean(filepath.Join(s.Root, rel))
		if abs != s.Root && !strings.HasPrefix(abs, s.Root+string(filepath.Separator)) {
			return "", fmt.Errorf("path %q escapes the sandbox", rel)
		}
		return abs, nil
	}

	if filepath.IsAbs(rel) {
		return filepath.Clean(rel), nil
	}
	if s.Root == "" {
		abs, err := filepath.Abs(rel)
		if err != nil {
			return "", err
		}
		return abs, nil
	}
	return filepath.Clean(filepath.Join(s.Root, rel)), nil
}

// Rel returns abs as a path relative to the sandbox root, for display. On any
// error it falls back to the cleaned absolute path.
func (s Sandbox) Rel(abs string) string {
	if rel, err := filepath.Rel(s.Root, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

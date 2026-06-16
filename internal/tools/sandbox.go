package tools

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Sandbox confines filesystem and shell tools to a single workspace directory.
// Every path an agent supplies is resolved relative to Root and verified to stay
// inside it, so a tool call can never read or write outside the workspace.
type Sandbox struct {
	Root string // absolute, cleaned workspace root
}

// NewSandbox builds a sandbox rooted at dir. dir is made absolute and cleaned;
// an empty dir yields a sandbox whose Resolve always fails (no tool can escape).
func NewSandbox(dir string) Sandbox {
	if dir == "" {
		return Sandbox{}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Sandbox{}
	}
	return Sandbox{Root: filepath.Clean(abs)}
}

// Ready reports whether the sandbox has a usable root.
func (s Sandbox) Ready() bool { return s.Root != "" }

// Resolve turns an agent-supplied (relative) path into an absolute path inside
// the sandbox. It rejects absolute inputs and any path that, after cleaning,
// would escape the root via "..". The empty path resolves to the root itself.
func (s Sandbox) Resolve(rel string) (string, error) {
	if !s.Ready() {
		return "", fmt.Errorf("filesystem sandbox is not configured")
	}
	rel = strings.TrimSpace(rel)
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed; use a path relative to the workspace root")
	}
	abs := filepath.Clean(filepath.Join(s.Root, rel))
	if abs != s.Root && !strings.HasPrefix(abs, s.Root+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes the workspace sandbox", rel)
	}
	return abs, nil
}

// Rel returns abs as a path relative to the sandbox root, for display. On any
// error it falls back to the cleaned absolute path.
func (s Sandbox) Rel(abs string) string {
	if rel, err := filepath.Rel(s.Root, abs); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(abs)
}

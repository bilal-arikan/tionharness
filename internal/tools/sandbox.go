package tools

import (
	"fmt"
	"path/filepath"
	"runtime"
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
// When Confined is true every resolved path is kept inside Root: ".." escapes
// and absolute paths that point outside Root are rejected, while an absolute path
// that already resolves inside Root is honoured. This is used for the narrow
// config-dir sandbox, where an agent may only touch its own
// <workspace>/config/ files, and for autonomous turns re-confined to their
// working dir.
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
//
// The cleaned path is then run through filepath.EvalSymlinks so Root is stored in
// the same spelling the OS will report for paths opened underneath it. This is a
// correctness/consistency fix, not a security boundary: when Root itself is a
// symlink, the lexical Root and the real path of a file opened through it diverge,
// so an agent naming that same file by its real absolute path was rejected by the
// underRoot check even though it is the very file Root points at. Resolving Root
// once also keeps this boundary in step with the API-side one (internal/api.underDir).
//
// It does NOT resolve links *inside* Root — a symlink or junction below Root still
// passes the lexical underRoot check and lets the OS open a target outside Root.
// Confinement remains a lexical boundary; closing that hole is a separate change.
//
// EvalSymlinks failing is an expected, benign case (Root may not exist yet, e.g. a
// working dir created later), so the cleaned lexical path is kept as-is rather than
// dropping the root. On Windows this is also the normal outcome for a junction:
// EvalSymlinks reports junctions verbatim instead of following them.
func cleanRoot(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	cleaned := filepath.Clean(abs)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return filepath.Clean(resolved)
}

// isSlashRooted reports a Windows path written with a root slash but no drive.
// filepath.IsAbs rejects this spelling on Windows; without an explicit check it
// would be silently joined to Root. UNC paths remain valid absolute paths.
func isSlashRooted(path string) bool {
	if runtime.GOOS != "windows" || len(path) == 0 || (path[0] != '/' && path[0] != '\\') {
		return false
	}
	if len(path) >= 2 && (path[1] == '/' || path[1] == '\\') {
		return false
	}
	return !filepath.IsAbs(path)
}

// ntNamespacePrefixes are the Windows NT / device namespace spellings that let a
// caller address the filesystem without going through normal Win32 path parsing.
// filepath.Clean does not normalise them, so a confined sandbox must reject them
// explicitly rather than relying on the Root prefix comparison happening to miss.
var ntNamespacePrefixes = []string{`\\?\`, `\\.\`, `\??\`}

// rejectWindowsPathTricks rejects Windows path spellings that survive
// filepath.Clean and would therefore make the confinement boundary depend on how
// Root itself happens to be spelled:
//
//   - NT / device namespace prefixes (\\?\, \\.\, \??\, including \\?\UNC\) and
//     GLOBALROOT device paths. These bypass Win32 path normalisation entirely.
//   - Alternate data streams (file.txt:stream). Clean leaves the ":" in place, so
//     the stream suffix also defeats extension allowlists built on filepath.Ext.
//
// Both checks are Windows-only: on other platforms these spellings carry no
// special meaning and ":" is a legal filename character.
func rejectWindowsPathTricks(path string) error {
	if runtime.GOOS != "windows" || path == "" {
		return nil
	}
	slashed := strings.ReplaceAll(path, "/", `\`)
	for _, prefix := range ntNamespacePrefixes {
		if strings.HasPrefix(slashed, prefix) {
			return fmt.Errorf("path %q uses the Windows NT/device namespace (%s), which is not allowed in a confined sandbox: use a plain drive-qualified path (C:\\...) or a path relative to the working directory", path, prefix)
		}
	}
	for _, elem := range strings.Split(slashed, `\`) {
		if strings.EqualFold(elem, "GLOBALROOT") {
			return fmt.Errorf("path %q addresses a GLOBALROOT device path, which is not allowed in a confined sandbox: use a plain drive-qualified path (C:\\...) or a path relative to the working directory", path)
		}
	}
	// VolumeName consumes the legitimate "C:" drive letter (and the UNC
	// \\server\share prefix); any ":" left over is a stream separator.
	if strings.Contains(slashed[len(filepath.VolumeName(slashed)):], ":") {
		return fmt.Errorf("path %q names an NTFS alternate data stream, which is not allowed in a confined sandbox", path)
	}
	return nil
}

// underRoot reports whether abs is Root itself or a descendant of it. On Windows
// the comparison is case-insensitive (NTFS is), matching the API-side boundary in
// internal/api.underDir so both boundaries behave identically on one platform.
func underRoot(abs, root string) bool {
	prefix := root + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(abs, root) ||
			(len(abs) >= len(prefix) && strings.EqualFold(abs[:len(prefix)], prefix))
	}
	return abs == root || strings.HasPrefix(abs, prefix)
}

// Ready reports whether the sandbox has a configured base directory.
func (s Sandbox) Ready() bool { return s.Root != "" }

// Resolve turns an agent-supplied path into an absolute path.
//
// Unconfined (default): absolute inputs are honoured as-is; relative inputs
// resolve against Root (or, when Root is empty, the process working directory);
// ".." escapes are allowed.
//
// Confined: requires a configured Root and keeps every path inside it — a ".."
// escape or an absolute path outside Root is rejected, while an absolute path that
// resolves inside Root is honoured. The empty path resolves to Root. On Windows,
// NT/device namespace spellings and alternate data streams are rejected outright
// (see rejectWindowsPathTricks) so the boundary does not depend on how Root is
// spelled.
func (s Sandbox) Resolve(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	// Checked before the slash-rooted check so NT namespace and stream inputs get
	// their own diagnostic instead of the generic "not a valid absolute path" one.
	if s.Confined {
		if err := rejectWindowsPathTricks(rel); err != nil {
			return "", err
		}
	}
	if isSlashRooted(rel) {
		return "", fmt.Errorf("path %q is not a valid absolute path on this platform: use a drive-qualified path (C:\\...) or a path relative to the working directory; it was NOT resolved against the working root", rel)
	}

	if s.Confined {
		if !s.Ready() {
			return "", fmt.Errorf("filesystem sandbox is not configured")
		}
		// Confinement guards against ESCAPE, not against spelling a path absolutely.
		// An absolute path that resolves inside Root is safe, so honour it and let the
		// escape check below be the single boundary — rejecting in-Root absolutes only
		// forces autonomous agents into avoidable retries.
		var abs string
		if filepath.IsAbs(rel) {
			abs = filepath.Clean(rel)
		} else {
			abs = filepath.Clean(filepath.Join(s.Root, rel))
		}
		if !underRoot(abs, s.Root) {
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

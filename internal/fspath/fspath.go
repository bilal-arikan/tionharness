// Package fspath compares filesystem paths the way the host filesystem does.
//
// Windows (NTFS) and macOS (APFS/HFS+ in their default configuration) treat
// path names case-insensitively; Linux filesystems do not. Code that dedupes or
// bounds paths must agree with the OS, or the same directory reached as /Users/x/Repo
// and /users/x/repo is treated as two different places.
package fspath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CaseInsensitive reports whether the host's default filesystem compares path
// names case-insensitively.
func CaseInsensitive() bool { return caseInsensitive(runtime.GOOS) }

func caseInsensitive(goos string) bool { return goos == "windows" || goos == "darwin" }

// Equal reports whether a and b name the same path after cleaning, folding case
// on case-insensitive hosts. It is lexical only (no stat): use it for dedupe
// keys, not for security boundaries — see Within.
func Equal(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if CaseInsensitive() {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// Key normalises p for use as a map key: cleaned, and lower-cased on
// case-insensitive hosts.
func Key(p string) string {
	p = filepath.Clean(p)
	if CaseInsensitive() {
		return strings.ToLower(p)
	}
	return p
}

// Within reports whether target is dir itself or a descendant of it. Both are
// expected to be cleaned absolute paths.
//
// An exact (case-sensitive) prefix always matches. A match that only holds after
// case folding is accepted on Windows outright; on macOS a volume may be
// formatted case-sensitive, so the folded prefix is accepted only when it is the
// very same directory as dir on disk (os.SameFile) — otherwise /Root and a sibling
// /root on a case-sensitive volume would be confused.
func Within(dir, target string) bool {
	if target == dir {
		return true
	}
	prefix := dir + string(filepath.Separator)
	if strings.HasPrefix(target, prefix) {
		return true
	}
	if !CaseInsensitive() {
		return false
	}
	var head string
	switch {
	case strings.EqualFold(target, dir):
		head = target
	case len(target) >= len(prefix) && strings.EqualFold(target[:len(prefix)], prefix):
		head = target[:len(dir)]
	default:
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return sameFile(dir, head)
}

func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// FileModeFor is the permission a materialised bundled file (skill resource,
// hook-pack script) should get: executable when the content starts with a "#!"
// shebang, so `./scripts/run.sh` works on macOS/Linux after an import that only
// carried the bytes, plain 0644 otherwise. Windows ignores the exec bit.
func FileModeFor(data []byte) os.FileMode {
	if len(data) >= 2 && data[0] == '#' && data[1] == '!' {
		return 0o755
	}
	return 0o644
}

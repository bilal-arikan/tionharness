package tools

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSandboxRejectsSlashRootedWindowsPaths(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filepath semantics only")
	}
	sb := NewSandbox(t.TempDir())
	for _, path := range []string{"/etc/passwd", "/mnt/c/x", `\tmp\x`} {
		t.Run(path, func(t *testing.T) {
			got, err := sb.Resolve(path)
			if err == nil || got != "" {
				t.Fatalf("Resolve(%q) = %q, %v; want empty path and explicit error", path, got, err)
			}
		})
	}
}

func TestSandboxSlashRootedWindowsCheckExcludesUNC(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows filepath semantics only")
	}
	if isSlashRooted(`\\server\share\x`) {
		t.Fatal("UNC path was classified as slash-rooted without a drive")
	}
}

// A confined sandbox must keep every path inside Root while no longer punishing an
// agent for spelling an in-Root path absolutely: confinement guards against escape,
// not against the absolute form.
func TestConfinedSandbox_HonoursInRootAbsolute(t *testing.T) {
	root := t.TempDir()
	sb := NewConfinedSandbox(root)

	inRootAbs := filepath.Join(root, "backend", "internal", "x.go")
	got, err := sb.Resolve(inRootAbs)
	if err != nil {
		t.Fatalf("in-root absolute path rejected: %v", err)
	}
	if want := filepath.Clean(inRootAbs); got != want {
		t.Errorf("resolved = %q, want %q", got, want)
	}

	// A relative path still resolves against Root.
	rel, err := sb.Resolve(filepath.Join("backend", "x.go"))
	if err != nil {
		t.Fatalf("relative path rejected: %v", err)
	}
	if want := filepath.Join(root, "backend", "x.go"); rel != want {
		t.Errorf("relative resolved = %q, want %q", rel, want)
	}
}

// The escape boundary is unchanged: an absolute path OUTSIDE Root, and a ".."
// traversal, are both rejected.
func TestConfinedSandbox_RejectsEscapes(t *testing.T) {
	root := t.TempDir()
	sb := NewConfinedSandbox(root)

	outsideAbs := filepath.Join(filepath.Dir(root), "sibling", "secret.go")
	if _, err := sb.Resolve(outsideAbs); err == nil {
		t.Errorf("out-of-root absolute path %q was allowed", outsideAbs)
	}
	if _, err := sb.Resolve(filepath.Join("..", "escape.go")); err == nil {
		t.Error("\"..\" traversal was allowed")
	}
}

// The Windows NT/device namespace and alternate data streams must be rejected by
// an explicit rule, not as a side effect of Root never being spelled that way: if
// cleanRoot ever normalised Root to \\?\ form the prefix comparison alone would
// invert. Every spelling below must fail regardless of what Root looks like.
func TestConfinedSandbox_RejectsWindowsPathTricks(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path namespace semantics only")
	}
	root := t.TempDir()
	sb := NewConfinedSandbox(root)

	// Strip the "C:" so each namespace spelling can address the very same file
	// inside Root that a plain path would reach.
	rootNoVolume := strings.TrimPrefix(root, filepath.VolumeName(root))

	cases := []struct {
		name string
		path string
	}{
		{"win32 file namespace", `\\?\` + root + `\x.go`},
		{"win32 device namespace", `\\.\` + root + `\x.go`},
		{"nt object namespace", `\??\` + root + `\x.go`},
		{"globalroot device", `\\?\GLOBALROOT\Device\HarddiskVolume3` + rootNoVolume + `\x.go`},
		{"unc via file namespace", `\\?\UNC\server\share\x.go`},
		{"plain unc", `\\server\share\x.go`},
		{"forward slash file namespace", `//?/` + root + `/x.go`},
		{"alternate data stream", filepath.Join(root, "file.txt") + ":stream"},
		{"alternate data stream on relative path", `file.txt:stream`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sb.Resolve(tc.path)
			if err == nil || got != "" {
				t.Fatalf("Resolve(%q) = %q, %v; want empty path and an error", tc.path, got, err)
			}
		})
	}
}

// The escape check compares against Root case-insensitively on Windows, matching
// the filesystem and the API-side boundary (internal/api.underDir). A path that
// differs from Root only in case is inside the sandbox, not an escape.
func TestConfinedSandbox_InRootCaseDiffersFromRoot(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive path comparison is Windows-only")
	}
	root := t.TempDir()
	sb := NewConfinedSandbox(root)

	lowered := filepath.Join(strings.ToLower(root), "x.go")
	got, err := sb.Resolve(lowered)
	if err != nil {
		t.Fatalf("in-root path with different casing rejected: %v", err)
	}
	if want := filepath.Clean(lowered); got != want {
		t.Errorf("resolved = %q, want %q", got, want)
	}

	if _, err := sb.Resolve(strings.ToLower(root)); err != nil {
		t.Errorf("Root itself with different casing rejected: %v", err)
	}
}

// The unconfined sandbox is deliberately boundary-free; the new rejections must
// not leak into it.
func TestUnconfinedSandbox_LeavesWindowsPathTricksAlone(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path namespace semantics only")
	}
	sb := NewSandbox(t.TempDir())
	if _, err := sb.Resolve(`\\?\C:\x.go`); err != nil {
		t.Errorf("unconfined sandbox rejected %q: %v", `\\?\C:\x.go`, err)
	}
}

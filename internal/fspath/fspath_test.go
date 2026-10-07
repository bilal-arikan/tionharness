package fspath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCaseInsensitiveHosts(t *testing.T) {
	for goos, want := range map[string]bool{"windows": true, "darwin": true, "linux": false, "freebsd": false} {
		if got := caseInsensitive(goos); got != want {
			t.Errorf("caseInsensitive(%q) = %v, want %v", goos, got, want)
		}
	}
}

func TestWithinExact(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "root")
	for _, target := range []string{dir, filepath.Join(dir, "a"), filepath.Join(dir, "a", "b.go")} {
		if !Within(dir, target) {
			t.Errorf("Within(%q, %q) = false, want true", dir, target)
		}
	}
	for _, target := range []string{dir + "x", filepath.Dir(dir), filepath.Join(dir+"x", "a")} {
		if Within(dir, target) {
			t.Errorf("Within(%q, %q) = true, want false", dir, target)
		}
	}
}

// A case-folded match counts only where the filesystem agrees it is the same
// directory: always on Windows, on macOS only when the folded spelling stats to the
// same directory, never on Linux.
func TestWithinFoldedCase(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Root")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	folded := filepath.Join(filepath.Dir(dir), "root", "x.go")
	got := Within(dir, folded)
	switch runtime.GOOS {
	case "linux":
		if got {
			t.Errorf("Within accepted a case-folded path on Linux")
		}
	case "windows":
		if !got {
			t.Errorf("Within rejected a case-folded path on Windows")
		}
	case "darwin":
		_, err := os.Stat(strings.TrimSuffix(folded, string(filepath.Separator)+"x.go"))
		if want := err == nil; got != want {
			t.Errorf("Within = %v, want %v (volume case-insensitive: %v)", got, want, want)
		}
	}
}

func TestEqualAndKey(t *testing.T) {
	a, b := filepath.Join("x", "Repo"), filepath.Join("x", "repo", ".")
	if got, want := Equal(a, b), CaseInsensitive(); got != want {
		t.Errorf("Equal(%q, %q) = %v, want %v", a, b, got, want)
	}
	if got, want := Key(a) == Key(b), CaseInsensitive(); got != want {
		t.Errorf("Key equality = %v, want %v", got, want)
	}
}

func TestFileModeFor(t *testing.T) {
	if got := FileModeFor([]byte("#!/bin/sh\necho hi\n")); got != 0o755 {
		t.Errorf("shebang script mode = %o, want 755", got)
	}
	if got := FileModeFor([]byte("# heading\n")); got != 0o644 {
		t.Errorf("plain file mode = %o, want 644", got)
	}
}

package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSubstitutePluginRoot(t *testing.T) {
	cases := []struct {
		name, goos, cmd, dir, want string
	}{
		{"safe dir verbatim", "darwin", "${CLAUDE_PLUGIN_ROOT}/hooks/run.sh --x", "/Users/a/.tionharness/hooks/p", "/Users/a/.tionharness/hooks/p/hooks/run.sh --x"},
		{"unquoted word quoted", "darwin", "${CLAUDE_PLUGIN_ROOT}/hooks/run.sh --x", "/Users/a/Library/Application Support/p", "'/Users/a/Library/Application Support/p/hooks/run.sh' --x"},
		{"argument word quoted", "linux", "python3 ${CLAUDE_PLUGIN_ROOT}/h.py", "/home/a b/p", "python3 '/home/a b/p/h.py'"},
		{"inside double quotes", "darwin", `node "${CLAUDE_PLUGIN_ROOT}/x.js"`, "/x y/$p", `node "/x y/\$p/x.js"`},
		{"inside single quotes", "linux", `sh -c '${CLAUDE_PLUGIN_ROOT}/x'`, "/x y/it's", `sh -c '/x y/it'\''s/x'`},
		{"windows leading word gets call operator", "windows", `${CLAUDE_PLUGIN_ROOT}\hooks\run.ps1 -A`, `C:\Users\Ad Soyad\p`, `& 'C:\Users\Ad Soyad\p\hooks\run.ps1' -A`},
		{"windows argument word", "windows", `node ${CLAUDE_PLUGIN_ROOT}\x.js`, `C:\Users\Ad Soyad\p`, `node 'C:\Users\Ad Soyad\p\x.js'`},
		{"windows safe backslash dir verbatim", "windows", `node ${CLAUDE_PLUGIN_ROOT}\x.js`, `C:\Users\a\p`, `node C:\Users\a\p\x.js`},
	}
	for _, c := range cases {
		if got := substitutePluginRoot(c.cmd, c.dir, c.goos); got != c.want {
			t.Errorf("%s:\n got  %s\n want %s", c.name, got, c.want)
		}
	}
}

// End to end on a POSIX host: a pack script under a directory with a space runs
// through bash after substitution — a plain replace would split the path.
func TestSubstitutePluginRootRunsWithSpaces(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell check")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not installed")
	}
	dir := filepath.Join(t.TempDir(), "Application Support", "pack")
	if err := os.MkdirAll(filepath.Join(dir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks", "run.sh"), []byte("#!/bin/sh\necho ran:$1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := substitutePluginRoot("${CLAUDE_PLUGIN_ROOT}/hooks/run.sh ok", dir, runtime.GOOS)
	out, err := exec.Command(bash, "-c", cmd).CombinedOutput()
	if err != nil || strings.TrimSpace(string(out)) != "ran:ok" {
		t.Fatalf("bash -c %q = %q, %v", cmd, out, err)
	}
}

package exttools

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// zvec-grep is an npm package: its `zg` shim lands in whichever global bin
// directory the npm that installed it owns, and that directory is not always on
// the backend's PATH. These tests pin the resolution order.

func TestZvecGrepCatalogEntry(t *testing.T) {
	tool := Find(ZvecGrepToolName)
	if tool == nil {
		t.Fatal("zg is missing from the catalog")
	}
	if tool.Wire != "mcp" {
		t.Errorf("wire = %q, want mcp", tool.Wire)
	}
	if got := tool.Repo(); got != "zvec-ai/zvec-grep" {
		t.Errorf("release feed = %q", got)
	}
	// The shared daemon holds native addons open; a package-manager run against it
	// fails half-applied on Windows, so the panel must never offer one.
	if tool.Update.Kind != UpdateManual || tool.Update.Note == "" {
		t.Errorf("update must be manual with instructions, got %+v", tool.Update)
	}
}

// writeExe creates a placeholder file at path, creating its directories.
func writeExe(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// offPath simulates a backend whose PATH does not contain zg, with an empty home
// and APPDATA so that only what the test creates can match. Returns the home dir.
func offPath(t *testing.T) string {
	t.Helper()
	t.Setenv("TIONHARNESS_ZG", "")
	isolateHome(t)
	t.Setenv("APPDATA", t.TempDir())
	orig := lookPath
	t.Cleanup(func() { lookPath = orig })
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	return home
}

func TestZvecGrepEnvOverride(t *testing.T) {
	home := offPath(t)
	exe := filepath.Join(t.TempDir(), zvecGrepExeName())
	writeExe(t, exe)
	t.Setenv("TIONHARNESS_ZG", exe)
	if found, p := Detect(ZvecGrepToolName); !found || p != exe {
		t.Errorf("Detect ignored TIONHARNESS_ZG: found=%v path=%q", found, p)
	}

	// A configured path that points at nothing means "not installed", even with a
	// real install sitting in an nvm directory.
	writeExe(t, filepath.Join(home, ".nvm", "versions", "node", "v24.18.1", "bin", zvecGrepExeName()))
	t.Setenv("TIONHARNESS_ZG", filepath.Join(t.TempDir(), "missing", zvecGrepExeName()))
	if got := zvecGrepExe(); got != "" {
		t.Errorf("broken override fell back to %q", got)
	}
}

func TestZvecGrepPathWins(t *testing.T) {
	home := offPath(t)
	writeExe(t, filepath.Join(home, ".nvm", "versions", "node", "v24.18.1", "bin", zvecGrepExeName()))
	onPath := filepath.Join(t.TempDir(), zvecGrepExeName())
	lookPath = func(string) (string, error) { return onPath, nil }
	if got := zvecGrepExe(); got != onPath {
		t.Errorf("zvecGrepExe = %q, want the PATH hit %q", got, onPath)
	}
}

// The measured case: zg installed with the nvm-managed npm, backend started from
// PowerShell, nvm's bin directory absent from PATH.
func TestZvecGrepFindsNewestNvmInstallOffPath(t *testing.T) {
	home := offPath(t)
	node := filepath.Join(home, ".nvm", "versions", "node")
	older := filepath.Join(node, "v24.2.0", "bin", zvecGrepExeName())
	newer := filepath.Join(node, "v24.18.1", "bin", zvecGrepExeName())
	writeExe(t, older)
	writeExe(t, newer)
	if found, p := Detect(ZvecGrepToolName); !found || p != newer {
		t.Errorf("Detect = (%v, %q), want the newest nvm install %q", found, p, newer)
	}
}

func TestZvecGrepPrefersNpmPrefixOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the APPDATA npm prefix exists on Windows only")
	}
	home := offPath(t)
	appData := t.TempDir()
	t.Setenv("APPDATA", appData)
	npmExe := filepath.Join(appData, "npm", "zg.cmd")
	writeExe(t, npmExe)
	writeExe(t, filepath.Join(home, ".nvm", "versions", "node", "v24.18.1", "bin", "zg.cmd"))
	if got := zvecGrepExe(); got != npmExe {
		t.Errorf("zvecGrepExe = %q, want %q", got, npmExe)
	}
}

func TestNodeVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v22.1.0", "v24.18.1", true},
		{"v24.2.0", "v24.18.1", true}, // numeric, not lexical
		{"v24.18.1", "v24.2.0", false},
		{"system", "v18.0.0", true}, // a real version always wins
		{"v18.0.0", "system", false},
	}
	for _, c := range cases {
		if got := nodeVersionLess(c.a, c.b); got != c.want {
			t.Errorf("nodeVersionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

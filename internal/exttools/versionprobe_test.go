package exttools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mkVenv builds a minimal venv skeleton (pyvenv.cfg + Scripts/bin with an
// interpreter and a console script) and returns the console script's path.
func mkVenv(t *testing.T, root string, withCfg bool) string {
	t.Helper()
	scripts := "bin"
	exe, py := "piper", "python"
	if runtime.GOOS == "windows" {
		scripts, exe, py = "Scripts", "piper.exe", "python.exe"
	}
	dir := filepath.Join(root, scripts)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if withCfg {
		if err := os.WriteFile(filepath.Join(root, "pyvenv.cfg"), []byte("home = C:\\Python313\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{exe, py} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, exe)
}

// piper's current wheel install has NO --version flag, so the default probe
// would park a permanent error in the panel. Inside a venv the probe must switch
// to that venv's interpreter and ask importlib.metadata instead.
func TestVersionProbeUsesVenvInterpreterForPiper(t *testing.T) {
	tool := Tool{Name: "piper", VersionArgs: []string{"--version"}}
	script := mkVenv(t, filepath.Join(t.TempDir(), ".venv"), true)

	probe, args := tool.VersionProbe(script)
	if probe == script {
		t.Fatal("venv install must be probed through the interpreter, not the console script")
	}
	if !strings.HasPrefix(filepath.Base(probe), "python") {
		t.Fatalf("probe = %q, want the venv python", probe)
	}
	if len(args) != 2 || args[0] != "-c" || !strings.Contains(args[1], "piper-tts") {
		t.Fatalf("args = %v, want a -c importlib.metadata expression for piper-tts", args)
	}
}

// The pyvenv.cfg guard is the whole reason this is safe: /usr/bin/piper sits next
// to /usr/bin/python3 without being a venv, and probing that interpreter would
// report on a piper-tts it does not have. Such a layout must fall back to the
// tool's own --version, which the legacy standalone binary does answer.
func TestVersionProbeFallsBackWithoutVenvMarker(t *testing.T) {
	tool := Tool{Name: "piper", VersionArgs: []string{"--version"}}
	script := mkVenv(t, filepath.Join(t.TempDir(), "usr"), false)

	probe, args := tool.VersionProbe(script)
	if probe != script {
		t.Fatalf("probe = %q, want the tool itself when there is no pyvenv.cfg", probe)
	}
	if len(args) != 1 || args[0] != "--version" {
		t.Fatalf("args = %v, want the catalog VersionArgs", args)
	}
}

// Every other tool keeps the plain contract, including the empty-args case that
// tells the caller to skip the probe entirely.
func TestVersionProbeIsIdentityForOtherTools(t *testing.T) {
	script := mkVenv(t, filepath.Join(t.TempDir(), ".venv"), true)

	// Same venv shape, different tool: the piper-specific rule must not leak.
	probe, args := Tool{Name: "rtk", VersionArgs: []string{"--version"}}.VersionProbe(script)
	if probe != script || len(args) != 1 || args[0] != "--version" {
		t.Fatalf("rtk probe = %q %v, want the tool with its own flag", probe, args)
	}

	if _, args := (Tool{Name: "some-tool"}).VersionProbe("/x/some-tool"); len(args) != 0 {
		t.Fatalf("args = %v, want empty so the caller skips the probe", args)
	}
}

// The catalog's piper entry must keep VersionArgs populated: it is what the
// legacy standalone install is probed with, and dropping it would silently
// disable version reporting for those hosts.
func TestPiperCatalogKeepsLegacyVersionFlag(t *testing.T) {
	tool := Find("piper")
	if tool == nil {
		t.Fatal("piper missing from catalog")
	}
	if len(tool.VersionArgs) == 0 {
		t.Fatal("piper must keep --version for the legacy standalone binary")
	}
	if tool.Update.Kind != UpdateManual || tool.Update.Note == "" {
		t.Fatal("piper update must stay manual with a note naming the pip command")
	}
}

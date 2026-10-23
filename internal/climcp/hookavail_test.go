package climcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHookExecutableMissing(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "present-tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for cmd, wantMissing := range map[string]bool{
		"sqz hook claude":                 true,
		"FOO=1 BAR=2 sqz hook":            true,
		"present-tool --flag":             false,
		"'present-tool' x":                false,
		tool + " x":                       false,
		filepath.Join(dir, "gone") + " x": true,
		"./scripts/hook.sh":               false, // relative to the CLI's cwd
		"echo hi >&2; exit 2":             false, // builtin
		"$HOME/bin/hook":                  false, // shell expansion
		"(cd /tmp && something)":          false, // subshell
		"":                                false,
	} {
		if _, got := HookExecutableMissing(cmd); got != wantMissing {
			t.Errorf("HookExecutableMissing(%q) = %v, want %v", cmd, got, wantMissing)
		}
	}
}

package proc_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exemptMarker opts a file out of the no-raw-exec rule. A file carrying it must
// explain in a comment why a visible console (or a hidden GUI window) is fine.
const exemptMarker = "exec-console-exempt"

// hideHelpers are the calls that make a child process console-less on Windows.
// A file that spawns a process must use one of them, or the packaged desktop
// binary (-H windowsgui, no console of its own) flashes a terminal window for
// every child it starts.
var hideHelpers = []string{
	"proc.Command(",
	"proc.CommandContext(",
	"proc.CommandContextNested(",
	"proc.Hide(",
	"proc.HideConsole(",
	"proc.HideNested(",
}

// TestNoRawExecCommand fails when a package spawns a process with a bare
// exec.Command / exec.CommandContext and never routes it through the proc
// helpers. This is the repo-wide guard behind the "no flashing terminal"
// behaviour: adding a new child process without hiding its console is caught
// here instead of by a user watching windows blink.
func TestNoRawExecCommand(t *testing.T) {
	root := repoRoot(t)
	var offenders []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "website", "frontend":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// The proc package IS the hiding layer — it necessarily calls exec directly.
		if filepath.Base(filepath.Dir(path)) == "proc" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(src)
		if !strings.Contains(text, "exec.Command") {
			return nil
		}
		if strings.Contains(text, exemptMarker) {
			return nil
		}
		for _, h := range hideHelpers {
			if strings.Contains(text, h) {
				return nil
			}
		}
		rel, _ := filepath.Rel(root, path)
		offenders = append(offenders, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	if len(offenders) > 0 {
		t.Errorf("these files spawn a process without hiding its console:\n  %s\n"+
			"use proc.Command/proc.CommandContext (console child), proc.CommandContextNested "+
			"(child that spawns its own children), proc.HideConsole (console child that shows a "+
			"dialog), or add the %q marker with a comment explaining why a visible window is correct",
			strings.Join(offenders, "\n  "), exemptMarker)
	}
}

// repoRoot walks up from the test's directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

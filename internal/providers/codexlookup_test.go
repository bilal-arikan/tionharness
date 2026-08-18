package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCodexCandidatePathsWindows(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\tester\AppData\Local`)
	t.Setenv("PROGRAMFILES", `C:\Program Files`)
	t.Setenv("APPDATA", `C:\Users\tester\AppData\Roaming`)

	got := codexCandidatePaths("windows")
	want := []string{
		filepath.Join(`C:\Users\tester\AppData\Local`, "Programs", "OpenAI", "Codex", "bin", "codex.exe"),
		filepath.Join(`C:\Program Files`, "OpenAI", "Codex", "bin", "codex.exe"),
		filepath.Join(`C:\Users\tester\AppData\Roaming`, "npm", "codex.cmd"),
	}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodexCandidatePathsWindowsEmptyEnvSkipped(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("PROGRAMFILES", `C:\Program Files`)
	t.Setenv("APPDATA", "")

	got := codexCandidatePaths("windows")
	want := []string{filepath.Join(`C:\Program Files`, "OpenAI", "Codex", "bin", "codex.exe")}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestCodexCandidatePathsWindowsAllEnvEmpty(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("PROGRAMFILES", "")
	t.Setenv("APPDATA", "")

	got := codexCandidatePaths("windows")
	if len(got) != 0 {
		t.Fatalf("candidates = %v, want empty", got)
	}
}

func TestCodexCandidatePathsLinux(t *testing.T) {
	t.Setenv("HOME", "/home/tester")

	got := codexCandidatePaths("linux")
	want := []string{
		filepath.Join("/home/tester", ".local", "bin", "codex"),
		"/usr/local/bin/codex",
	}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodexCandidatePathsLinuxEmptyHomeSkipped(t *testing.T) {
	t.Setenv("HOME", "")

	got := codexCandidatePaths("linux")
	want := []string{"/usr/local/bin/codex"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestCodexCandidatePathsDarwin(t *testing.T) {
	t.Setenv("HOME", "/Users/tester")

	got := codexCandidatePaths("darwin")
	want := []string{
		filepath.Join("/Users/tester", ".local", "bin", "codex"),
		"/usr/local/bin/codex",
		"/opt/homebrew/bin/codex",
	}
	if len(got) != len(want) {
		t.Fatalf("candidate count = %d, want %d (got %v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("candidate[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestCodexCandidatePathsUnknownOS(t *testing.T) {
	got := codexCandidatePaths("plan9")
	if len(got) != 0 {
		t.Fatalf("candidates = %v, want empty for unknown GOOS", got)
	}
}

func TestIsRegularFileResolvesRealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "codex.exe")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	if !isRegularFile(path) {
		t.Errorf("isRegularFile(%q) = false, want true for a real file", path)
	}
}

func TestIsRegularFileRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	if isRegularFile(dir) {
		t.Errorf("isRegularFile(%q) = true, want false for a directory", dir)
	}
}

func TestIsRegularFileRejectsMissingPath(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist")
	if isRegularFile(missing) {
		t.Errorf("isRegularFile(%q) = true, want false for a missing path", missing)
	}
}

func TestLookupCodexBinaryFallsBackToWindowsCandidate(t *testing.T) {
	// PATH is left as the real host PATH here: if this test host happens to
	// have a real "codex" on PATH, that is a legitimate higher-priority hit
	// and lookupCodexBinary is correct to return it instead of the fallback.
	// So this test only proves the candidate list itself resolves a real
	// file, not that lookupCodexBinary necessarily picks it - that would be
	// host-dependent and non-hermetic.
	dir := t.TempDir()
	binDir := filepath.Join(dir, "Programs", "OpenAI", "Codex", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("os.MkdirAll: %v", err)
	}
	binPath := filepath.Join(binDir, "codex.exe")
	if err := os.WriteFile(binPath, []byte(""), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	t.Setenv("LOCALAPPDATA", dir)
	t.Setenv("PROGRAMFILES", filepath.Join(dir, "no-such-program-files"))
	t.Setenv("APPDATA", filepath.Join(dir, "no-such-appdata"))

	candidates := codexCandidatePaths("windows")
	if len(candidates) == 0 || candidates[0] != binPath {
		t.Fatalf("first windows candidate = %v, want %q as first entry", candidates, binPath)
	}
	if !isRegularFile(candidates[0]) {
		t.Errorf("expected constructed candidate %q to resolve as a real file", candidates[0])
	}
}

package providers

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// lookupCodexBinary resolves the codex CLI binary path.
//
// Search order: PATH first (exec.LookPath), then a fixed list of well-known
// per-OS install locations. PATH always wins when it resolves — the fallback
// exists only because OpenAI's official Codex installer (Windows and some
// package-manager builds) drops the binary into a fixed directory without
// adding it to PATH, so a fresh install is otherwise invisible to us even
// though the user is logged in and ready to go.
//
// Returns "" if nothing resolves.
func lookupCodexBinary() string {
	if path, err := exec.LookPath("codex"); err == nil {
		return path
	}
	for _, candidate := range codexCandidatePaths(runtime.GOOS) {
		if isRegularFile(candidate) {
			return candidate
		}
	}
	return ""
}

// codexCandidatePaths returns the well-known codex install locations for
// goos, built from environment variables so no path is ever hardcoded to a
// specific user account. A candidate whose backing env var is empty is
// skipped rather than turned into a malformed path.
func codexCandidatePaths(goos string) []string {
	var candidates []string

	switch goos {
	case "windows":
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "Programs", "OpenAI", "Codex", "bin", "codex.exe"))
		}
		if programFiles := os.Getenv("PROGRAMFILES"); programFiles != "" {
			candidates = append(candidates, filepath.Join(programFiles, "OpenAI", "Codex", "bin", "codex.exe"))
		}
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "npm", "codex.cmd"))
		}
	case "darwin", "linux":
		if home := os.Getenv("HOME"); home != "" {
			candidates = append(candidates, filepath.Join(home, ".local", "bin", "codex"))
		}
		candidates = append(candidates, "/usr/local/bin/codex")
		if goos == "darwin" {
			candidates = append(candidates, "/opt/homebrew/bin/codex")
		}
	}

	return candidates
}

// isRegularFile reports whether path exists and is a regular file (not a
// directory). A stat error other than "not exists" is treated as absent
// rather than propagated — the caller only needs a yes/no for path
// candidates, never a reason.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

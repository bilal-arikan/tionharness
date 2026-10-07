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
		candidates = unixCLICandidates(goos, "codex")
	}

	return candidates
}

// lookupClaudeBinary resolves the claude CLI the same way lookupCodexBinary
// resolves codex: PATH first, then the well-known per-user install locations. The
// fallback matters on macOS, where an app started from Finder/Dock/launchd gets a
// minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin) that hides ~/.local/bin and
// Homebrew. Returns "" if nothing resolves.
func lookupClaudeBinary() string {
	if path, err := exec.LookPath("claude"); err == nil {
		return path
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		var candidates []string
		if home := os.Getenv("HOME"); home != "" {
			// The native installer's private copy (`claude migrate-installer`).
			candidates = append(candidates, filepath.Join(home, ".claude", "local", "claude"))
		}
		for _, candidate := range append(candidates, unixCLICandidates(runtime.GOOS, "claude")...) {
			if isRegularFile(candidate) {
				return candidate
			}
		}
	}
	return ""
}

// unixCLICandidates lists the usual macOS/Linux install locations of an npm- or
// installer-distributed CLI named name: per-user bin dirs (native installers,
// npm's ~/.npm-global prefix, bun, volta) first, then the system-wide ones
// (/usr/local/bin, Homebrew). Per-user candidates are skipped when HOME is unset.
func unixCLICandidates(goos, name string) []string {
	var candidates []string
	if home := os.Getenv("HOME"); home != "" {
		for _, dir := range [][]string{{".local", "bin"}, {".npm-global", "bin"}, {".bun", "bin"}, {".volta", "bin"}} {
			candidates = append(candidates, filepath.Join(append(append([]string{home}, dir...), name)...))
		}
	}
	candidates = append(candidates, "/usr/local/bin/"+name)
	switch goos {
	case "darwin":
		candidates = append(candidates, "/opt/homebrew/bin/"+name)
	case "linux":
		candidates = append(candidates, "/home/linuxbrew/.linuxbrew/bin/"+name)
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

package providers

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// lookupOpenCodeBinary resolves the opencode CLI the same way lookupCodexBinary
// resolves codex: PATH first, then the well-known per-user install locations
// (npm/bun/volta global bins, Homebrew). Returns "" if nothing resolves.
func lookupOpenCodeBinary() string {
	if path, err := exec.LookPath("opencode"); err == nil {
		return path
	}
	for _, candidate := range opencodeCandidatePaths(runtime.GOOS) {
		if isRegularFile(candidate) {
			return candidate
		}
	}
	return ""
}

// opencodeCandidatePaths returns the well-known opencode install locations for
// goos, built from environment variables so no path is hardcoded to a user.
func opencodeCandidatePaths(goos string) []string {
	var candidates []string
	switch goos {
	case "windows":
		if appData := os.Getenv("APPDATA"); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "npm", "opencode.cmd"))
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "Programs", "opencode", "opencode.exe"))
		}
	case "darwin", "linux":
		if home := os.Getenv("HOME"); home != "" {
			// The official install script's default target.
			candidates = append(candidates, filepath.Join(home, ".opencode", "bin", "opencode"))
		}
		candidates = append(candidates, unixCLICandidates(goos, "opencode")...)
	}
	return candidates
}

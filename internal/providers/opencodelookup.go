package providers

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// lookupOpenCodeBinary resolves the opencode CLI the same way lookupCodexBinary
// resolves codex: PATH first, then the well-known per-user install locations
// (npm/bun/volta global bins, Homebrew, the desktop app's bundled CLI). Returns
// "" if nothing resolves.
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

// opencodeDesktopID is the OpenCode desktop app's bundle identifier; its
// per-user app-data directory holds the CLI it ships, one folder per version
// (e.g. %APPDATA%\ai.opencode.desktop\cli\2.2.5-x64\opencode-cli.exe).
const opencodeDesktopID = "ai.opencode.desktop"

// opencodeCandidatePaths returns the well-known opencode install locations for
// goos, built from environment variables so no path is hardcoded to a user.
func opencodeCandidatePaths(goos string) []string {
	var candidates []string
	switch goos {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			candidates = append(candidates, filepath.Join(appData, "npm", "opencode.cmd"))
		}
		if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "Programs", "opencode", "opencode.exe"))
		}
		if appData != "" {
			candidates = append(candidates, opencodeDesktopCLIs(filepath.Join(appData, opencodeDesktopID, "cli"), "opencode-cli.exe")...)
		}
	case "darwin", "linux":
		home := os.Getenv("HOME")
		if home != "" {
			// The official install script's default target.
			candidates = append(candidates, filepath.Join(home, ".opencode", "bin", "opencode"))
		}
		candidates = append(candidates, unixCLICandidates(goos, "opencode")...)
		// The desktop app's bundled CLI under the platform app-data directory
		// (same layout as on Windows; verified there only).
		if home != "" {
			dataDir := filepath.Join(home, "Library", "Application Support")
			if goos == "linux" {
				dataDir = os.Getenv("XDG_DATA_HOME")
				if dataDir == "" {
					dataDir = filepath.Join(home, ".local", "share")
				}
			}
			candidates = append(candidates, opencodeDesktopCLIs(filepath.Join(dataDir, opencodeDesktopID, "cli"), "opencode-cli")...)
		}
	}
	return candidates
}

// opencodeDesktopCLIs lists cliRoot/<version>/<exe> for every version folder,
// newest version first. The desktop app replaces the folder on upgrade, so the
// list is read fresh on every lookup.
func opencodeDesktopCLIs(cliRoot, exe string) []string {
	entries, err := os.ReadDir(cliRoot)
	if err != nil {
		return nil
	}
	type versioned struct {
		path string
		ver  [3]int
	}
	var found []versioned
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		found = append(found, versioned{path: filepath.Join(cliRoot, e.Name(), exe), ver: parseVersionPrefix(e.Name())})
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i].ver, found[j].ver
		for k := range a {
			if a[k] != b[k] {
				return a[k] > b[k]
			}
		}
		return found[i].path > found[j].path
	})
	out := make([]string, len(found))
	for i, f := range found {
		out[i] = f.path
	}
	return out
}

// parseVersionPrefix reads the leading "MAJOR.MINOR.PATCH" of a folder name
// such as "2.2.5-x64"; missing or non-numeric parts count as 0.
func parseVersionPrefix(name string) [3]int {
	var v [3]int
	core, _, _ := strings.Cut(name, "-")
	for i, part := range strings.SplitN(core, ".", 3) {
		n, err := strconv.Atoi(part)
		if err != nil {
			break
		}
		v[i] = n
	}
	return v
}

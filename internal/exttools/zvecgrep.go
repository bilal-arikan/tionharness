package exttools

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// zvecGrepExeName is the launcher npm installs for the zvec-grep CLI. On Windows
// that is the .cmd shim: the extensionless `zg` next to it is a POSIX sh script
// only Git Bash can run, and CreateProcess refuses it.
func zvecGrepExeName() string {
	if runtime.GOOS == "windows" {
		return "zg.cmd"
	}
	return "zg"
}

// zvecGrepExe resolves the zvec-grep CLI: TIONHARNESS_ZG, then PATH, then the npm
// global bin directories a process started outside the user's shell does not
// have on its PATH.
//
// The fallback is measured, not speculative (2026-09-14): zg was installed with
// the nvm-managed npm, whose bin directory (~/.nvm/versions/node/<v>/bin) only Git
// Bash's profile puts on PATH. A backend started from PowerShell resolved node
// from C:\Program Files\nodejs and reported zg as missing while every Git Bash
// session used it — the "panel contradicts the running system" failure
// SetPathOverride exists to prevent.
func zvecGrepExe() string {
	if p := strings.TrimSpace(os.Getenv("TIONHARNESS_ZG")); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
		// Same rule as Detect's overrides: a configured path that points at nothing
		// means "not installed", never "use whatever else is around".
		return ""
	}
	if p, err := lookPath("zg"); err == nil {
		return p
	}
	for _, c := range zvecGrepCandidates() {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// zvecGrepCandidates lists the npm global bin locations to probe, most specific
// first: npm's default prefix on Windows (%APPDATA%\npm), then every nvm-managed
// Node, newest version first.
func zvecGrepCandidates() []string {
	exe := zvecGrepExeName()
	var out []string
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			out = append(out, filepath.Join(appData, "npm", exe))
		}
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return out
	}
	versions, _ := filepath.Glob(filepath.Join(home, ".nvm", "versions", "node", "v*"))
	sort.Slice(versions, func(i, j int) bool {
		return nodeVersionLess(filepath.Base(versions[j]), filepath.Base(versions[i]))
	})
	for _, v := range versions {
		out = append(out, filepath.Join(v, "bin", exe))
	}
	return out
}

// nodeVersionLess orders nvm version directory names (v22.1.0 < v24.18.1). A name
// that carries no version sorts first, so a real version always wins.
func nodeVersionLess(a, b string) bool {
	aMaj, aMin, aPatch, aOK := ParseVersion(a)
	bMaj, bMin, bPatch, bOK := ParseVersion(b)
	if aOK != bOK {
		return !aOK
	}
	if aMaj != bMaj {
		return aMaj < bMaj
	}
	if aMin != bMin {
		return aMin < bMin
	}
	return aPatch < bPatch
}

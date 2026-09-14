package api

import (
	"os"
	"path/filepath"
	"strings"
)

// Where to look for codex plugin marketplaces already installed on this machine.
//
// Scope is deliberately narrow. A codex home also holds auth.json, secrets/ and
// live session databases, so this never walks the home itself — it probes a
// short list of KNOWN marketplace locations and reads nothing but each one's
// marketplace.json. Anything else in those directories is ignored.

// codexMarketplaceSearchRoots returns candidate marketplace roots, in the order
// they should be offered. Non-existent paths are dropped, so the result is
// exactly what is really installed.
func codexMarketplaceSearchRoots() []string {
	var roots []string
	add := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		if fi, err := os.Stat(filepath.Join(path, codexMarketplaceManifestRel)); err == nil && !fi.IsDir() {
			roots = append(roots, path)
		}
	}

	home := codexUserHome()
	if home != "" {
		// The marketplace the Codex desktop app stages for its bundled plugins.
		bundled := filepath.Join(home, ".tmp", "bundled-marketplaces")
		for _, entry := range readDirNames(bundled) {
			add(filepath.Join(bundled, entry))
		}
		// A marketplace the user added themselves, if it lives under the home.
		userMarketplaces := filepath.Join(home, "marketplaces")
		for _, entry := range readDirNames(userMarketplaces) {
			add(filepath.Join(userMarketplaces, entry))
		}
	}

	// The primary-runtime plugins ship in the per-user cache rather than the home.
	if cache, err := os.UserCacheDir(); err == nil {
		runtimes := filepath.Join(cache, "codex-runtimes")
		for _, runtime := range readDirNames(runtimes) {
			plugins := filepath.Join(runtimes, runtime, "plugins")
			for _, entry := range readDirNames(plugins) {
				add(filepath.Join(plugins, entry))
			}
		}
	}
	return roots
}

// codexUserHome resolves the user's own codex home ($CODEX_HOME, else ~/.codex).
// This is the ambient install, NOT a workspace's isolated home.
func codexUserHome() string {
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex")
}

// readDirNames lists dir's immediate subdirectory names; a missing or unreadable
// directory yields nothing, since absence is the normal case here.
func readDirNames(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

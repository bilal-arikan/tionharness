package exttools

import (
	"os"
	"path/filepath"
	"strings"
)

// codexNpmPackage is the npm package that ships the Codex CLI launcher.
const codexNpmPackage = "@openai/codex"

// codexUpdateSpec picks the one-click update for the codex install at path, or
// reports false when the install's owner cannot be identified (a standalone
// binary, a cargo build, a copy someone dropped into ~/.local/bin) — the catalog's
// static manual note covers those.
//
// The owner is read from the install itself rather than assumed, because running
// the wrong manager does not update anything: `npm install -g` against a Homebrew
// cask lays down a SECOND codex that may or may not win on PATH, and the panel
// would keep reporting the old one.
//
// npm installs get an explicit --prefix taken from the install location. The npm
// first on the backend's PATH can belong to a different Node than the one that
// installed codex (nvm next to a Homebrew/installer Node); without --prefix it
// would update its own global tree and leave the binary TionHarness runs behind.
func codexUpdateSpec(path string) (UpdateSpec, bool) {
	if prefix := npmGlobalPrefix(path, codexNpmPackage); prefix != "" {
		return UpdateSpec{
			Kind:    UpdateCommand,
			Command: "npm",
			Args:    []string{"install", "-g", "--prefix", prefix, codexNpmPackage + "@latest"},
		}, true
	}
	if strings.Contains(resolvedSlashPath(path), "/Caskroom/codex/") {
		return UpdateSpec{Kind: UpdateCommand, Command: "brew", Args: []string{"upgrade", "--cask", "codex"}}, true
	}
	return UpdateSpec{}, false
}

// npmGlobalPrefix returns the npm global prefix that owns the launcher at path
// for package pkg, or "" when the launcher is not an npm global install.
//
// Two layouts exist:
//   - POSIX: <prefix>/bin/codex is a symlink into
//     <prefix>/lib/node_modules/@openai/codex/…
//   - Windows: <prefix>\codex.cmd is a shim file (not a symlink) sitting next to
//     <prefix>\node_modules\@openai\codex
func npmGlobalPrefix(path, pkg string) string {
	if path == "" {
		return ""
	}
	marker := "/node_modules/" + pkg + "/"
	resolved := resolvedSlashPath(path)
	if i := strings.Index(resolved, marker); i >= 0 {
		root := filepath.FromSlash(resolved[:i])
		if filepath.Base(root) == "lib" {
			root = filepath.Dir(root)
		}
		return root
	}
	dir := filepath.Dir(path)
	if st, err := os.Stat(filepath.Join(dir, "node_modules", filepath.FromSlash(pkg))); err == nil && st.IsDir() {
		return dir
	}
	return ""
}

// resolvedSlashPath follows symlinks (falling back to path when that fails) and
// normalises separators, so substring checks work on every platform.
func resolvedSlashPath(path string) string {
	if r, err := filepath.EvalSymlinks(path); err == nil {
		path = r
	}
	return filepath.ToSlash(path)
}

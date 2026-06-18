package market

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultsFS holds the starter packs shipped with SwarmGo. They are seeded into
// the global market dir on startup so every workspace can browse them.
//
//go:embed defaults
var defaultsFS embed.FS

// EnsureDefaults writes the bundled starter packs into dir, creating only the
// ones that are missing. Existing files are never overwritten, so user edits and
// re-published packs survive; a deleted default reappears on next start. A blank
// dir is a no-op. Mirrors skills.EnsureDefaults.
func EnsureDefaults(dir string) error {
	if dir == "" {
		return nil
	}
	return fs.WalkDir(defaultsFS, "defaults", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel("defaults", p)
		if relErr != nil {
			return relErr
		}
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if _, statErr := os.Stat(dest); statErr == nil {
			return nil // already present — leave as-is
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
			return mkErr
		}
		data, readErr := defaultsFS.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(dest, data, 0o644)
	})
}

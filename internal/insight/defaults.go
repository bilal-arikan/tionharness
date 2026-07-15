package insight

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultsFS holds the built-in lenses shipped with TionSwarm. They are seeded
// into a workspace's lens dir on startup so every workspace inherits the baseline
// scan intents. Unlike the skills seed, this is intentionally simple: it writes a
// default only when it is MISSING and never overwrites a user edit (frontmatter-
// aware body refresh can come later — see _Docs/60 TODO).
//
//go:embed defaults
var defaultsFS embed.FS

// LensesDirRel is the store-root-relative directory that holds the workspace's
// editable lens files.
var LensesDirRel = filepath.Join("insight", "lenses")

// LensesDir returns the absolute lens directory under a store root (db.Root()).
func LensesDir(root string) string { return filepath.Join(root, LensesDirRel) }

// EnsureDefaults writes any missing built-in lens into dir. Existing files are
// left untouched (user edits are preserved). A blank dir is a no-op.
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
			return nil // already present — never overwrite a user edit
		}
		embedded, readErr := defaultsFS.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(dest, embedded, 0o644)
	})
}

package skills

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// defaultsFS holds the built-in skills shipped with SwarmGo. They are seeded
// into the global skills dir on startup so every workspace inherits them.
//
//go:embed defaults
var defaultsFS embed.FS

// DefaultSkillSlugs returns the slugs of the shipped default skills (the
// subdirectories under defaults/), so callers can seed new agents with the
// baseline SwarmGo skill set. Sorted for a stable order. Single source of truth:
// the embedded defaults tree.
func DefaultSkillSlugs() []string {
	entries, err := fs.ReadDir(defaultsFS, "defaults")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// EnsureDefaults writes the built-in default skills into dir, creating only the
// ones that are missing. Existing files are never overwritten, so user edits and
// the access toggle survive; a deleted default reappears on next start (these
// are shipped, baseline skills). A blank dir is a no-op.
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

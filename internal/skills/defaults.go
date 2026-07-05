package skills

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// defaultsFS holds the built-in skills shipped with TionSwarm. They are seeded
// into the global skills dir on startup so every workspace inherits them.
//
//go:embed defaults
var defaultsFS embed.FS

// shippedManifestName is the sidecar, at the root of the seed dir, that records
// the sha256 of the SHIPPED content last written for each default file. It lets
// EnsureDefaults tell an unmodified prior-shipped copy (safe to refresh) from a
// user-edited one (must be preserved) — the version-aware re-seed. It is a
// dotfile, so the skill store (which scans subdirs for SKILL.md) never treats it
// as a skill.
const shippedManifestName = ".shipped-versions.json"

// DefaultSkillSlugs returns the slugs of the shipped default skills (the
// subdirectories under defaults/), so callers can seed new agents with the
// baseline TionSwarm skill set. Sorted for a stable order. Single source of truth:
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

// sha256Hex returns the lowercase hex sha256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// loadShippedManifest reads the shipped-version sidecar from dir. A missing or
// unreadable manifest yields an empty map (first run of the version-aware code),
// so every existing on-disk file is treated as unknown-provenance and preserved.
func loadShippedManifest(dir string) map[string]string {
	m := map[string]string{}
	data, err := os.ReadFile(filepath.Join(dir, shippedManifestName))
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	if m == nil {
		m = map[string]string{}
	}
	return m
}

// saveShippedManifest writes the shipped-version sidecar back to dir (best effort).
func saveShippedManifest(dir string, m map[string]string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, shippedManifestName), data, 0o644)
}

// EnsureDefaults writes the built-in default skills into dir, VERSION-AWARE:
//
//   - A missing file is written and its shipped hash recorded.
//   - An on-disk file identical to the embedded one is left as-is (its hash is
//     recorded, so future ships know it is pristine).
//   - An on-disk file that DIFFERS from the embedded one but matches the hash of
//     the PREVIOUSLY shipped version (recorded in the manifest) is unmodified by
//     the user, so it is REFRESHED to the new embedded content. This is what makes
//     shipped skill updates reach existing installs (the old EnsureDefaults never
//     overwrote, so evolved defaults went stale on any machine that had run before).
//   - An on-disk file that differs from BOTH the embedded and the last-shipped hash
//     (or has no manifest entry) is treated as a USER EDIT and preserved untouched.
//
// A blank dir is a no-op. Bootstrapping note: on the first run of this version-
// aware code the manifest is absent, so pre-existing files are all treated as
// user edits (preserved); the manifest then seeds itself for every file that
// currently matches the embedded content, so subsequent ships can refresh them.
func EnsureDefaults(dir string) error {
	if dir == "" {
		return nil
	}
	manifest := loadShippedManifest(dir)
	changed := false

	walkErr := fs.WalkDir(defaultsFS, "defaults", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel("defaults", p)
		if relErr != nil {
			return relErr
		}
		key := filepath.ToSlash(rel) // stable manifest key across OSes
		dest := filepath.Join(dir, filepath.FromSlash(rel))

		embedded, readErr := defaultsFS.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		hEmbed := sha256Hex(embedded)

		onDisk, statErr := os.ReadFile(dest)
		if statErr != nil {
			// Missing (or unreadable) — write it fresh and record the shipped hash.
			if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
				return mkErr
			}
			if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
				return wErr
			}
			manifest[key] = hEmbed
			changed = true
			return nil
		}

		hDisk := sha256Hex(onDisk)
		if hDisk == hEmbed {
			// Already current — just make sure the manifest records it as pristine.
			if manifest[key] != hEmbed {
				manifest[key] = hEmbed
				changed = true
			}
			return nil
		}

		// Differs from the embedded content: refresh only if it is the untouched
		// previously-shipped version; otherwise it is a user edit and we leave it.
		if prev, ok := manifest[key]; ok && prev == hDisk {
			if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
				return wErr
			}
			manifest[key] = hEmbed
			changed = true
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	if changed {
		return saveShippedManifest(dir, manifest)
	}
	return nil
}

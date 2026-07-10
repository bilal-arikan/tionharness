package skills

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
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

// EnsureDefaults writes the built-in default skills into dir, VERSION-AWARE and
// (for SKILL.md files) FRONTMATTER-AWARE:
//
//   - A missing file is written and its shipped hashes recorded.
//   - An on-disk file identical to the embedded one is left as-is (its hashes
//     are recorded, so future ships know it is pristine).
//   - An on-disk file whose WHOLE content matches the previously shipped hash
//     (manifest Files) is unmodified by the user → fully refreshed, frontmatter
//     included. This is what makes shipped skill updates reach existing installs.
//   - A SKILL.md whose whole hash matches nothing but whose BODY matches the
//     embedded or previously shipped body (manifest Bodies) only had its
//     FRONTMATTER tuned (the app rewrites access/group/visibility markers in
//     place). The frontmatter is user config and is always preserved; a
//     previously-shipped pristine body is refreshed to the new embedded body
//     underneath it. Without this, one visibility toggle froze the file forever.
//   - Anything else is a USER EDIT and is preserved untouched.
//
// A blank dir is a no-op. Bootstrapping: with no manifest (or a legacy flat
// one), files are preserved; hashes then seed themselves for every file whose
// content (or body) currently matches the embedded tree, so subsequent ships
// can refresh them.
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
		isSkill := d.Name() == "SKILL.md"
		var embedBody, hEmbedBody string
		if isSkill {
			embedBody = skillBody(embedded)
			hEmbedBody = sha256Hex([]byte(embedBody))
		}
		recordShipped := func() {
			if manifest.Files[key] != hEmbed {
				manifest.Files[key] = hEmbed
				changed = true
			}
			if isSkill && manifest.Bodies[key] != hEmbedBody {
				manifest.Bodies[key] = hEmbedBody
				changed = true
			}
		}

		onDisk, statErr := os.ReadFile(dest)
		if statErr != nil {
			// Missing (or unreadable) — write it fresh and record the shipped hashes.
			if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
				return mkErr
			}
			if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
				return wErr
			}
			recordShipped()
			return nil
		}

		hDisk := sha256Hex(onDisk)
		if hDisk == hEmbed {
			// Already current — just make sure the manifest records it as pristine.
			recordShipped()
			return nil
		}

		// Untouched previously-shipped WHOLE file → full refresh (this path also
		// ships frontmatter changes, so it stays first).
		if prev, ok := manifest.Files[key]; ok && prev == hDisk {
			if wErr := os.WriteFile(dest, embedded, 0o644); wErr != nil {
				return wErr
			}
			recordShipped()
			return nil
		}

		if !isSkill {
			return nil // user edit — preserve
		}

		// Frontmatter-aware path: compare bodies alone so shipped BODY updates
		// still land under a user-tuned frontmatter. The frontmatter itself is
		// never touched here.
		diskFM, diskBody := splitFrontmatter(string(onDisk))
		hDiskBody := sha256Hex([]byte(diskBody))
		if hDiskBody == hEmbedBody {
			// Body already current — only the frontmatter differs. Track the body
			// as pristine so the NEXT shipped body update can refresh it.
			if manifest.Bodies[key] != hEmbedBody {
				manifest.Bodies[key] = hEmbedBody
				changed = true
			}
			return nil
		}
		if prev, ok := manifest.Bodies[key]; ok && prev == hDiskBody {
			// Pristine previously-shipped body under user frontmatter → refresh
			// the body, keep the frontmatter verbatim.
			if wErr := os.WriteFile(dest, rebuildSkillFile(diskFM, embedBody), 0o644); wErr != nil {
				return wErr
			}
			manifest.Bodies[key] = hEmbedBody
			changed = true
			return nil
		}
		// Body edited by the user → preserve.
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

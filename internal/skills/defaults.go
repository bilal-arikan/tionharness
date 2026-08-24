package skills

import (
	"embed"
	"io/fs"
	"sort"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/seed"
)

// defaultsFS holds the built-in skills shipped with TionHarness. They are seeded
// into the global skills dir on startup so every workspace inherits them.
//
//go:embed defaults
var defaultsFS embed.FS

// DefaultSkillSlugs returns the slugs of the shipped default skills (the
// subdirectories under defaults/), so callers can seed new agents with the
// baseline TionHarness skill set. Sorted for a stable order. Single source of truth:
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

// seedConfig describes the shipped skill tree to the shared seeder. SKILL.md is
// the body-aware file: its frontmatter is USER config (the app rewrites the
// access/group/visibility markers in place), so a shipped body update must land
// underneath whatever frontmatter is currently there rather than replacing it.
func seedConfig(dir string) seed.Config {
	return seed.Config{
		FS:        defaultsFS,
		Root:      "defaults",
		Dir:       dir,
		BodyAware: func(name string) bool { return name == "SKILL.md" },
		Body:      func(content []byte) string { return skillBody(content) },
		Merge: func(onDisk, embedded []byte) []byte {
			fmText, _ := splitFrontmatter(string(onDisk))
			return rebuildSkillFile(fmText, skillBody(embedded))
		},
	}
}

// EnsureDefaults writes the built-in default skills into dir, version-aware and
// (for SKILL.md) frontmatter-aware. The rules — and why guessing at "did the user
// edit this?" is replaced by a shipped-hash ledger — live in package seed; this
// only supplies the skill-specific split (frontmatter = user config, body = ours).
// A blank dir is a no-op.
func EnsureDefaults(dir string) error {
	return seed.Ensure(seedConfig(dir))
}

// DefaultFileRel is the path of a skill's SKILL.md inside the defaults tree.
func DefaultFileRel(slug string) string {
	if slug == "" || strings.ContainsAny(slug, `/\`) {
		return ""
	}
	return slug + "/SKILL.md"
}

// RestoreDefault overwrites a shipped skill's SKILL.md with its embedded default,
// discarding local changes, and records it as pristine so future ships refresh it
// automatically. Errors when the slug names no shipped skill.
func RestoreDefault(dir, slug string) error {
	rel := DefaultFileRel(slug)
	if rel == "" {
		return fs.ErrNotExist
	}
	return seed.Restore(seedConfig(dir), rel)
}

// HasDefault reports whether a slug is one of the shipped default skills.
func HasDefault(slug string) bool {
	rel := DefaultFileRel(slug)
	// A blank rel would open the defaults DIRECTORY, which succeeds.
	return rel != "" && seed.HasDefault(seedConfig(""), rel)
}

// DefaultState classifies a skill's file against its shipped default (see
// seed.State). seed.StateNone for a user-authored or imported skill.
func DefaultState(dir, slug string) seed.State {
	rel := DefaultFileRel(slug)
	if rel == "" {
		return seed.StateNone
	}
	return seed.Status(seedConfig(dir), rel)
}

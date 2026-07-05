package skills

import (
	"path"

	"github.com/bilal-arikan/tionswarm/internal/fetch"
)

// Skill discovery in a directory tree (a GitHub repo, a Claude Code plugin, or a
// local folder) where each skill lives in its own sub-folder with a SKILL.md.
// Acquisition (tarball download / local walk) lives in internal/fetch; the generic
// MULTI-skill collection import now lives in internal/ingest. This file keeps only
// the grouping helper the SINGLE-skill importer (import.go) shares. (SK-IMP2/3)

// discoveredSkill is one skill found inside a tree: its SKILL.md text, its bundled
// files (keyed by path relative to the skill folder), and the folder's path relative
// to the tree root.
type discoveredSkill struct {
	relPath string            // skill folder path within the tree ("" = root)
	raw     string            // SKILL.md content
	files   map[string][]byte // bundled resources, relpath -> content
}

// groupSkills adapts the generic fetch.Group output (SKILL.md marker) to
// discoveredSkill, preserving nested resources via deepest-owner grouping.
func groupSkills(tree fetch.Tree, prefix string) []discoveredSkill {
	groups := fetch.GroupByMarker(tree, prefix, "SKILL.md")
	out := make([]discoveredSkill, 0, len(groups))
	for _, g := range groups {
		out = append(out, discoveredSkill{relPath: g.RelPath, raw: string(g.Marker), files: g.Files})
	}
	return out
}

// suggestSlug derives a slug for a discovered skill: the folder's base name,
// falling back to the frontmatter name, then "skill".
func suggestSlug(relPath, name string) string {
	base := path.Base(relPath)
	if relPath == "" {
		base = ""
	}
	slug := slugify(base)
	if slug == "" {
		slug = slugify(name)
	}
	if slug == "" {
		slug = "skill"
	}
	return slug
}

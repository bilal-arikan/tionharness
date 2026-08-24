package ingest

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/fetch"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// skillAdapter detects Claude Code skills (SKILL.md marker folders) in a tree and
// converts each into a skill HarnessPack, preserving nested bundled resources.
type skillAdapter struct{}

func (skillAdapter) Kind() string { return market.KindSkill }

func (skillAdapter) Scan(tree fetch.Tree, prefix, baseURL string) []Discovered {
	var out []Discovered
	for _, g := range fetch.GroupByMarker(tree, prefix, "SKILL.md") {
		raw := string(g.Marker)
		name := strings.TrimSpace(skills.FrontmatterField(raw, "name"))
		desc := skills.FrontmatterField(raw, "description")
		icon := skills.FrontmatterField(raw, "icon")
		color := skills.FrontmatterField(raw, "color")
		slug := skills.SuggestSlug(g.RelPath, name)

		// Render once (shared=false) to surface structural mapping warnings in the
		// preview (context:fork, unsupported keys, slash-command args…).
		_, prev := skills.RenderImportedSkill(raw, "", false, "")

		relPath := g.RelPath
		files := g.Files
		out = append(out, Discovered{
			Key:         market.KindSkill + ":" + relPath,
			Kind:        market.KindSkill,
			Slug:        slug,
			Name:        name,
			Description: desc,
			RelPath:     relPath,
			Files:       sortedKeys(files),
			Warnings:    prev.Warnings,
			build: func(opts Options) (market.Pack, error) {
				finalSlug := applyPrefix(skills.Slugify(opts.SlugPrefix), slug)
				body, res := skills.RenderImportedSkill(raw, itemURL(baseURL, relPath), opts.Shared, opts.Group)
				dispName := res.Name
				if dispName == "" {
					dispName = name
				}
				return market.BuildSkillPack(finalSlug, dispName, desc, icon, color, body, "", 0, files)
			},
		})
	}
	return out
}

package ingest

import (
	"path"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/fetch"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/skills"
)

// commandAdapter detects Claude Code slash commands (Markdown files under a
// commands/ dir) and converts each into a SKILL pack — SwarmGo has no "command"
// entity, so a command becomes a loadable skill (its body is the instructions) with
// the slug taken from the filename. TOML commands (.toml) are not parsed and are
// silently skipped (only Markdown commands are imported).
type commandAdapter struct{}

func (commandAdapter) Kind() string { return market.KindSkill }

func (commandAdapter) Scan(tree fetch.Tree, prefix, baseURL string) []Discovered {
	files := fetch.FindFiles(tree, prefix, func(n string) bool {
		return strings.HasSuffix(strings.ToLower(n), ".md")
	})
	var out []Discovered
	for _, p := range files {
		if !strings.EqualFold(path.Base(path.Dir(p)), "commands") {
			continue // only Markdown files directly inside a commands/ directory
		}
		raw := string(tree[p])
		base := strings.TrimSuffix(path.Base(p), path.Ext(p))
		name := strings.TrimSpace(skills.FrontmatterField(raw, "name"))
		if name == "" {
			name = base
		}
		desc := skills.FrontmatterField(raw, "description")
		body := skills.FrontmatterBody(raw)
		slug := skills.Slugify(base)

		var warnings []string
		if skills.FrontmatterField(raw, "argument-hint") != "" || strings.Contains(body, "$ARGUMENTS") {
			warnings = append(warnings, "slash-command arguments won't be substituted — imported as a loadable skill")
		}

		relPath := p
		out = append(out, Discovered{
			Key:         market.KindSkill + ":" + relPath,
			Kind:        market.KindSkill,
			Slug:        slug,
			Name:        name,
			Description: desc,
			RelPath:     relPath,
			Warnings:    warnings,
			build: func(opts Options) (market.Pack, error) {
				finalSlug := applyPrefix(skills.Slugify(opts.SlugPrefix), slug)
				content := synthSkillMD(name, desc, itemURL(baseURL, relPath), opts.Shared, body)
				return market.BuildSkillPack(finalSlug, name, desc, "", "", content, "", 0, nil)
			},
		})
	}
	return out
}

// synthSkillMD assembles a SwarmGo SKILL.md from a command's parts (commands carry no
// SKILL.md of their own).
func synthSkillMD(name, desc, sourceURL string, shared bool, body string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: " + yamlInline(name) + "\n")
	if desc != "" {
		b.WriteString("description: " + yamlInline(desc) + "\n")
	}
	if shared {
		b.WriteString("access: shared\n")
	}
	if sourceURL != "" {
		b.WriteString("source_url: " + yamlInline(sourceURL) + "\n")
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(body))
	b.WriteString("\n")
	return b.String()
}

// yamlInline double-quotes a scalar value when it contains characters that would
// break a bare YAML scalar.
func yamlInline(v string) string {
	v = strings.ReplaceAll(v, "\n", " ")
	if strings.ContainsAny(v, ":#\"'") || strings.TrimSpace(v) != v {
		return "\"" + strings.ReplaceAll(v, "\"", "\\\"") + "\""
	}
	return v
}

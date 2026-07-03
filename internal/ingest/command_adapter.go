package ingest

import (
	"path"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/fetch"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/skills"
)

// commandAdapter detects Claude Code slash commands (Markdown OR TOML files under a
// commands/ dir) and converts each into a SKILL pack — SwarmGo has no "command"
// entity, so a command becomes a loadable skill (its body is the instructions) with
// the slug taken from the filename. Markdown commands carry YAML frontmatter; TOML
// commands carry `description`/`prompt` keys (parsed without a TOML dependency).
type commandAdapter struct{}

func (commandAdapter) Kind() string { return market.KindSkill }

func (commandAdapter) Scan(tree fetch.Tree, prefix, baseURL string) []Discovered {
	files := fetch.FindFiles(tree, prefix, func(n string) bool {
		ln := strings.ToLower(n)
		return strings.HasSuffix(ln, ".md") || strings.HasSuffix(ln, ".toml")
	})
	var out []Discovered
	for _, p := range files {
		if !strings.EqualFold(path.Base(path.Dir(p)), "commands") {
			continue // only files directly inside a commands/ directory
		}
		raw := string(tree[p])
		base := strings.TrimSuffix(path.Base(p), path.Ext(p))
		name, desc, body := parseCommand(p, raw, base)
		if strings.TrimSpace(body) == "" {
			continue // nothing to import
		}
		slug := skills.Slugify(base)

		var warnings []string
		if commandHasArgs(raw, body) {
			warnings = append(warnings, "slash-command arguments won't be substituted — imported as a loadable skill")
		}

		relPath := p
		nm, ds, bd := name, desc, body
		out = append(out, Discovered{
			Key:         market.KindSkill + ":" + relPath,
			Kind:        market.KindSkill,
			Slug:        slug,
			Name:        nm,
			Description: ds,
			RelPath:     relPath,
			Warnings:    warnings,
			build: func(opts Options) (market.Pack, error) {
				finalSlug := applyPrefix(skills.Slugify(opts.SlugPrefix), slug)
				content := synthSkillMD(nm, ds, itemURL(baseURL, relPath), opts.Shared, opts.Group, bd)
				return market.BuildSkillPack(finalSlug, nm, ds, "", "", content, "", 0, nil)
			},
		})
	}
	return out
}

// parseCommand extracts (name, description, body) from a command file, handling both
// Markdown (YAML frontmatter + body) and TOML (description/prompt keys). The name
// defaults to the filename base.
func parseCommand(p, raw, base string) (name, desc, body string) {
	name = base
	if strings.EqualFold(path.Ext(p), ".toml") {
		kv := parseSimpleTOML(raw)
		if n := strings.TrimSpace(kv["name"]); n != "" {
			name = n
		}
		desc = strings.TrimSpace(kv["description"])
		body = firstNonEmpty(kv["prompt"], kv["body"], kv["command"], kv["content"])
		return name, desc, body
	}
	// Markdown command.
	if n := strings.TrimSpace(skills.FrontmatterField(raw, "name")); n != "" {
		name = n
	}
	desc = skills.FrontmatterField(raw, "description")
	body = skills.FrontmatterBody(raw)
	return name, desc, body
}

// commandHasArgs reports whether a command template uses argument placeholders that
// SwarmGo won't substitute when the command is imported as a skill.
func commandHasArgs(raw, body string) bool {
	if strings.Contains(body, "$ARGUMENTS") || strings.Contains(body, "{{args}}") {
		return true
	}
	return skills.FrontmatterField(raw, "argument-hint") != ""
}

// firstNonEmpty returns the first trimmed-non-empty string.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// synthSkillMD assembles a SwarmGo SKILL.md from a command's parts (commands carry no
// SKILL.md of their own). group, when non-empty, namespaces the command into a single
// Skills-UI group alongside the rest of the same import.
func synthSkillMD(name, desc, sourceURL string, shared bool, group, body string) string {
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
	if g := strings.TrimSpace(group); g != "" {
		b.WriteString("group: " + yamlInline(g) + "\n")
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

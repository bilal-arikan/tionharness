package ingest

import (
	"path"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/fetch"
	"github.com/bilal-arikan/swarmgo/internal/market"
	"github.com/bilal-arikan/swarmgo/internal/skills"
)

// agentAdapter detects Claude Code subagents (Markdown files under an agents/ dir
// with a `name` frontmatter) and converts each into an agent SwarmPack. The body
// becomes the agent's soul (system prompt); tools map to allowed_tools. The CC model
// is NOT mapped (provider/model differ) — a warning prompts the user to set it.
type agentAdapter struct{}

func (agentAdapter) Kind() string { return market.KindAgent }

func (agentAdapter) Scan(tree fetch.Tree, prefix, baseURL string) []Discovered {
	mdFiles := fetch.FindFiles(tree, prefix, func(n string) bool {
		return strings.HasSuffix(strings.ToLower(n), ".md")
	})
	var out []Discovered
	for _, p := range mdFiles {
		if !strings.EqualFold(path.Base(path.Dir(p)), "agents") {
			continue // only files directly inside an agents/ directory
		}
		raw := string(tree[p])
		name := strings.TrimSpace(skills.FrontmatterField(raw, "name"))
		if name == "" {
			continue // not a subagent definition (e.g. a README)
		}
		desc := skills.FrontmatterField(raw, "description")
		tools := skills.FrontmatterList(raw, "tools", "allowed-tools", "allowed_tools")
		model := skills.FrontmatterField(raw, "model")
		body := skills.FrontmatterBody(raw)
		slug := skills.Slugify(name)

		var warnings []string
		if model != "" {
			warnings = append(warnings, "CC model \""+model+"\" not mapped — set provider/model after install")
		}

		relPath := p
		out = append(out, Discovered{
			Key:         market.KindAgent + ":" + relPath,
			Kind:        market.KindAgent,
			Slug:        slug,
			Name:        name,
			Description: desc,
			RelPath:     relPath,
			Warnings:    warnings,
			build: func(opts Options) (market.Pack, error) {
				return market.Pack{
					Schema:      market.SchemaV1,
					ID:          market.KindAgent + "." + slug,
					Kind:        market.KindAgent,
					Name:        name,
					Description: desc,
					Version:     "1.0.0",
					Payload: market.Payload{Agent: &market.AgentPayload{
						Name:         name,
						Soul:         body,
						AllowedTools: strings.Join(tools, ", "),
					}},
				}, nil
			},
		})
	}
	return out
}

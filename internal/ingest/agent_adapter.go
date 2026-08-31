package ingest

import (
	"path"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/fetch"
	"github.com/bilal-arikan/tionharness/internal/market"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// agentAdapter detects Claude Code subagents (Markdown files under an agents/ dir
// with a `name` frontmatter) and converts each into an agent HarnessPack. The body
// becomes the agent's soul (system prompt); tools map to allowed_tools. The CC model
// is NOT mapped (provider/model differ) — a warning prompts the user to set it.
type agentAdapter struct{}

func (agentAdapter) Kind() string { return market.KindAgent }

// mapCCModel translates a Claude Code subagent `model:` value into a TionHarness
// (provider, model) pair. CC subagents name a model family (haiku/sonnet/opus) or
// "inherit"; TionHarness needs a concrete provider+model. It targets the keyless
// `claude-cli` provider (works out of the box, no API key) with the canonical model
// id for that family. An empty/"inherit" value leaves both blank (agent uses the
// workspace default). An unrecognised value (a non-Anthropic model, or a dated id we
// don't normalise) leaves both blank and returns a warning so the user sets it.
func mapCCModel(cc string) (provider, model, warn string) {
	s := strings.ToLower(strings.TrimSpace(cc))
	if s == "" || s == "inherit" || s == "default" {
		return "", "", ""
	}
	switch {
	case strings.Contains(s, "opus"):
		return "claude-cli", "claude-opus-4-8", ""
	case strings.Contains(s, "sonnet"):
		return "claude-cli", "claude-sonnet-5", ""
	case strings.Contains(s, "haiku"):
		return "claude-cli", "claude-haiku-4-5-20251001", ""
	case strings.Contains(s, "fable"):
		return "claude-cli", "claude-fable-5", ""
	}
	return "", "", "CC model \"" + cc + "\" not recognised — set provider/model after install"
}

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
		ccModel := skills.FrontmatterField(raw, "model")
		body := skills.FrontmatterBody(raw)
		slug := skills.Slugify(name)

		provider, model, modelWarn := mapCCModel(ccModel)
		// A CC subagent file names no reasoning tier, so the pack has to say which
		// one it means (an empty ThinkingLevel is no longer a valid agent value).
		// Only mapCCModel's recognised branch knows the provider kind; when it
		// leaves the provider blank the kind is still unresolved here — the
		// registry decides it at install time — so the level stays blank too and
		// market_install's explicitThinkingLevel resolves both together. The rule
		// itself is db.LegacyThinkingLevelFor's, called rather than copied.
		var thinkingLevel string
		if provider != "" {
			thinkingLevel = db.LegacyThinkingLevelFor(provider)
		}
		var warnings []string
		if modelWarn != "" {
			warnings = append(warnings, modelWarn)
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
						Name:          name,
						Soul:          body,
						Provider:      provider,
						Model:         model,
						ThinkingLevel: thinkingLevel,
						AllowedTools:  strings.Join(tools, ", "),
					}},
				}, nil
			},
		})
	}
	return out
}

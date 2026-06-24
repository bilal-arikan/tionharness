package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// SkillHit is one skill_search result row: enough for the agent to decide whether
// to load the skill's full body with use_skill.
type SkillHit struct {
	Slug        string
	Description string
	WhenToUse   string
}

// SkillSearchLibrary is the minimal view the skill_search tool needs. Implemented
// by an adapter over *skills.Store that restricts results to the skills the agent
// may actually load (assigned + shared/on-demand).
type SkillSearchLibrary interface {
	// SearchSkills returns matching skills (capped at limit; <=0 → a sane default).
	SearchSkills(query string, limit int) []SkillHit
}

const skillSearchDefaultLimit = 20

// SkillSearchTool lets an agent discover skills that are NOT advertised in the
// per-turn catalog — on-demand and conditional (paths-gated) skills — so the
// catalog can stay lean even with hundreds of skills installed. Results are
// loaded with use_skill. (SK-2)
type SkillSearchTool struct {
	lib SkillSearchLibrary
}

// NewSkillSearchTool constructs the skill_search tool over a skill library.
func NewSkillSearchTool(lib SkillSearchLibrary) SkillSearchTool { return SkillSearchTool{lib: lib} }

func (SkillSearchTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "skill_search",
		Description: "Search the full skill library by keyword to find a reusable skill that is not " +
			"listed in your \"Available Skills\" prompt block (on-demand or conditional skills are " +
			"kept out of the prompt to save context). Returns matching slugs + summaries; load one " +
			"with use_skill. Use this when a task seems to need a skill you don't see advertised.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": { "type": "string", "description": "Keywords to match against skill slug, name, description and when-to-use (all terms must match)." },
    "limit": { "type": "integer", "description": "Max results (default 20)." }
  },
  "required": ["query"],
  "additionalProperties": false
}`),
	}
}

func (t SkillSearchTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("skill_search", err)
	}
	if t.lib == nil {
		return "", fmt.Errorf("skills are not available in this context")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = skillSearchDefaultLimit
	}
	hits := t.lib.SearchSkills(strings.TrimSpace(in.Query), limit)
	if len(hits) == 0 {
		return fmt.Sprintf("No skills match %q.", in.Query), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Found %d skill(s). Load one with use_skill <slug>:\n", len(hits))
	for _, h := range hits {
		fmt.Fprintf(&b, "- `%s` — %s", h.Slug, h.Description)
		if h.WhenToUse != "" {
			fmt.Fprintf(&b, " (when: %s)", h.WhenToUse)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()), nil
}

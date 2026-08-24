package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// SkillValidation mirrors skills.SkillValidation without importing the skills
// package (keeps the tools layer dependency-light). The agent-side adapter
// converts the real result into this shape.
type SkillValidation struct {
	Found    bool
	Tier     string
	Path     string
	Valid    bool
	Errors   []string
	Warnings []string
}

// SkillValidator validates a skill's SKILL.md by slug. Implemented agent-side over
// the skills store.
type SkillValidator interface {
	ValidateSkill(slug string) SkillValidation
}

// skillValidateInput is the ask shape for the skill_validate tool.
type skillValidateInput struct {
	SkillSlug string `json:"skillSlug"`
}

// SkillValidateTool checks a workspace/global skill's SKILL.md for the problems
// that break loading or advertising (missing file, malformed frontmatter, missing
// name/description, empty body, bad slug). Read-only; pairs with create_skill /
// update_skill so an agent can self-check a skill it just authored.
type SkillValidateTool struct{ v SkillValidator }

// NewSkillValidateTool constructs the skill_validate tool.
func NewSkillValidateTool(v SkillValidator) SkillValidateTool { return SkillValidateTool{v: v} }

func (SkillValidateTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "skill_validate",
		Description: "Validate a skill's SKILL.md (slug hygiene, frontmatter name/description, non-empty body). " +
			"Use after authoring or editing a skill to catch errors before it is advertised or loaded. Read-only.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "skillSlug": { "type": "string", "description": "The skill slug (its directory name) to validate." }
  },
  "required": ["skillSlug"],
  "additionalProperties": false
}`),
	}
}

func (t SkillValidateTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.v == nil {
		return "", fmt.Errorf("skill validation is not available in this workspace")
	}
	in, err := parseInput[skillValidateInput]("skill_validate", input)
	if err != nil {
		return "", err
	}
	slug := strings.TrimSpace(in.SkillSlug)
	if slug == "" {
		return "", fmt.Errorf("skillSlug is required")
	}

	res := t.v.ValidateSkill(slug)
	var b strings.Builder
	if res.Valid {
		fmt.Fprintf(&b, "VALID — skill %q (%s tier) passed validation.", slug, tierOr(res.Tier))
	} else if !res.Found {
		fmt.Fprintf(&b, "INVALID — skill %q not found.", slug)
	} else {
		fmt.Fprintf(&b, "INVALID — skill %q (%s tier) has %d error(s):", slug, tierOr(res.Tier), len(res.Errors))
	}
	for _, e := range res.Errors {
		fmt.Fprintf(&b, "\n- %s", e)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(&b, "\n(warning) %s", w)
	}
	return b.String(), nil
}

func tierOr(t string) string {
	if t == "" {
		return "unknown"
	}
	return t
}

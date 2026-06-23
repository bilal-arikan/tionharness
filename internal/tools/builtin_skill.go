package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// SkillLibrary is the minimal view of the skill store the use_skill tool needs.
// Implemented by *skills.Store; kept as an interface here to avoid importing the
// skills package into tools (and to keep the tool unit-testable).
type SkillLibrary interface {
	// Body returns a skill's full markdown instructions for the given slug.
	Body(slug string) (string, error)
}

// UseSkillTool loads a named skill's full instructions on demand. The skill
// catalog (slug + summary) is advertised in the system prompt; the body stays on
// disk until the agent calls this tool — keeping the context lean (lazy loading).
type UseSkillTool struct {
	lib SkillLibrary
}

// NewUseSkillTool constructs the use_skill tool over a skill library.
func NewUseSkillTool(lib SkillLibrary) UseSkillTool { return UseSkillTool{lib: lib} }

func (UseSkillTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "use_skill",
		Description: "Load the full instructions of a reusable skill by its slug. The available " +
			"skills are listed in your system prompt under \"Available Skills\". Call this BEFORE " +
			"acting on a task that matches a skill, then follow the returned instructions.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "slug": { "type": "string", "description": "The skill slug exactly as shown in the Available Skills list." }
  },
  "required": ["slug"],
  "additionalProperties": false
}`),
	}
}

func (t UseSkillTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var in struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErrFor("use_skill", err)
	}
	slug := strings.TrimSpace(in.Slug)
	if slug == "" {
		return "", fmt.Errorf("slug is required")
	}
	if t.lib == nil {
		return "", fmt.Errorf("skills are not available in this context")
	}
	body, err := t.lib.Body(slug)
	if err != nil {
		return "", err
	}
	if body == "" {
		return fmt.Sprintf("Skill %q has no instructions.", slug), nil
	}
	return fmt.Sprintf("# Skill: %s\n\n%s", slug, body), nil
}

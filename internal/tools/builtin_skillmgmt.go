package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// SkillWriter is the minimal write view of the skill store the skill-management
// tools need. Implemented by an adapter over *skills.Store in the agent package
// (kept as an interface here so tools need not import the skills package).
type SkillWriter interface {
	CreateSkill(slug, name, description, whenToUse, body string, shared bool) error
	DeleteSkill(slug string) error
}

// Skill self-management tools let an agent author and remove reusable skills in
// its workspace — so an agent can distil a repeatable procedure into a named
// skill that it (and other agents) can later load via use_skill. Skills are
// workspace-tier markdown files; created skills appear in the catalog for the
// next turn.

// ---- create_skill ----

// CreateSkillTool writes a new workspace skill.
type CreateSkillTool struct{ w SkillWriter }

// NewCreateSkillTool constructs create_skill over a skill writer.
func NewCreateSkillTool(w SkillWriter) CreateSkillTool { return CreateSkillTool{w: w} }

func (CreateSkillTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "create_skill",
		Description: "Create a reusable workspace skill: a named set of markdown instructions that agents can later load with use_skill. Provide a slug (kebab-case id), a name, a description, an optional whenToUse hint (when an agent should reach for it), and the body (the full markdown instructions). The skill is available from the next turn. Returns the slug.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"slug":{"type":"string","description":"Kebab-case identifier, e.g. \"weekly-report\""},
				"name":{"type":"string","description":"Human-readable name"},
				"description":{"type":"string","description":"One-line summary shown in the catalog"},
				"whenToUse":{"type":"string","description":"Optional: when an agent should use this skill"},
				"body":{"type":"string","description":"The full markdown instructions"},
				"shared":{"type":"boolean","description":"Share across all agents in the workspace (default true)"}
			},
			"required":["slug","name","body"],
			"additionalProperties":false
		}`),
	}
}

func (t CreateSkillTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.w == nil {
		return "", fmt.Errorf("skill authoring is not available in this context")
	}
	var in struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		WhenToUse   string `json:"whenToUse"`
		Body        string `json:"body"`
		Shared      *bool  `json:"shared"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Slug = strings.TrimSpace(in.Slug)
	in.Name = strings.TrimSpace(in.Name)
	if in.Slug == "" || in.Name == "" || strings.TrimSpace(in.Body) == "" {
		return "", fmt.Errorf("slug, name and body are required")
	}
	shared := true
	if in.Shared != nil {
		shared = *in.Shared
	}
	if err := t.w.CreateSkill(in.Slug, in.Name, in.Description, in.WhenToUse, in.Body, shared); err != nil {
		return "", fmt.Errorf("create skill: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"slug": in.Slug, "action": "created"})
	return string(b), nil
}

// ---- delete_skill ----

// DeleteSkillTool removes a workspace skill by slug.
type DeleteSkillTool struct{ w SkillWriter }

// NewDeleteSkillTool constructs delete_skill over a skill writer.
func NewDeleteSkillTool(w SkillWriter) DeleteSkillTool { return DeleteSkillTool{w: w} }

func (DeleteSkillTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "delete_skill",
		Description: "Delete a workspace skill by its slug. Only workspace-tier skills can be removed (global/bundled skills are protected by the store).",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{"slug":{"type":"string","description":"The skill slug to delete"}},
			"required":["slug"],
			"additionalProperties":false
		}`),
	}
}

func (t DeleteSkillTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.w == nil {
		return "", fmt.Errorf("skill authoring is not available in this context")
	}
	var in struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", fmt.Errorf("invalid arguments: %w", err)
	}
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Slug == "" {
		return "", fmt.Errorf("slug is required")
	}
	if err := t.w.DeleteSkill(in.Slug); err != nil {
		return "", fmt.Errorf("delete skill: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"slug": in.Slug, "action": "deleted"})
	return string(b), nil
}

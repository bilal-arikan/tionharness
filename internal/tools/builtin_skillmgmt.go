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
	// UpdateSkill edits an existing workspace skill in place. Each field is a
	// pointer: nil leaves the current value untouched (partial update), so the
	// caller can change just the body without resupplying the rest.
	UpdateSkill(slug string, name, description, whenToUse, body *string, shared *bool) error
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
		return "", argErr(err)
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

// ---- update_skill ----

// UpdateSkillTool edits an existing workspace skill in place.
type UpdateSkillTool struct{ w SkillWriter }

// NewUpdateSkillTool constructs update_skill over a skill writer.
func NewUpdateSkillTool(w SkillWriter) UpdateSkillTool { return UpdateSkillTool{w: w} }

func (UpdateSkillTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name:        "update_skill",
		Description: "Edit an existing workspace skill in place by slug. Pass only the fields to change — name, description, whenToUse, body (full markdown, replaces the old one), or shared. Omitted fields keep their current value. The slug (folder) is immutable; only workspace-tier skills can be edited. Prefer this over delete_skill + create_skill. Takes effect from the next turn. Returns the slug.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"slug":{"type":"string","description":"The skill slug to edit (see use_skill / the catalog)"},
				"name":{"type":"string","description":"New human-readable name"},
				"description":{"type":"string","description":"New one-line summary"},
				"whenToUse":{"type":"string","description":"New when-to-use hint"},
				"body":{"type":"string","description":"New full markdown instructions (replaces the old body)"},
				"shared":{"type":"boolean","description":"Share across all agents in the workspace"}
			},
			"required":["slug"],
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			// Replace just the body; everything else untouched.
			json.RawMessage(`{"slug":"weekly-report","body":"# Weekly report\n\nUpdated steps..."}`),
			// Rename + retarget without resupplying the body.
			json.RawMessage(`{"slug":"weekly-report","name":"Weekly status report","whenToUse":"Every Friday before standup"}`),
		},
	}
}

func (t UpdateSkillTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.w == nil {
		return "", fmt.Errorf("skill authoring is not available in this context")
	}
	var in struct {
		Slug        string  `json:"slug"`
		Name        *string `json:"name"`
		Description *string `json:"description"`
		WhenToUse   *string `json:"whenToUse"`
		Body        *string `json:"body"`
		Shared      *bool   `json:"shared"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Slug == "" {
		return "", fmt.Errorf("slug is required")
	}
	if in.Name == nil && in.Description == nil && in.WhenToUse == nil && in.Body == nil && in.Shared == nil {
		return "", fmt.Errorf("nothing to update — provide at least one of: name, description, whenToUse, body, shared")
	}
	if err := t.w.UpdateSkill(in.Slug, in.Name, in.Description, in.WhenToUse, in.Body, in.Shared); err != nil {
		return "", fmt.Errorf("update skill: %w", err)
	}
	b, _ := json.Marshal(map[string]string{"slug": in.Slug, "action": "updated"})
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
		return "", argErr(err)
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

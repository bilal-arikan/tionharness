package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bilal-arikan/tionharness/internal/providers"
)

// SkillWriter is the minimal write view of the skill store the skill-management
// tools need. Implemented by an adapter over *skills.Store in the agent package
// (kept as an interface here so tools need not import the skills package).
type SkillWriter interface {
	CreateSkill(slug, name, description, whenToUse, group, body string, shared bool) error
	// UpdateSkill edits an existing workspace skill in place. Each field is a
	// pointer: nil leaves the current value untouched (partial update), so the
	// caller can change just the body without resupplying the rest.
	UpdateSkill(slug string, name, description, whenToUse, group, body *string, shared *bool) error
	DeleteSkill(slug string) error
	// ImportSkill imports a Claude Code skill from source ("local"|"github") at
	// location (a directory path or a github.com URL) into the workspace tier,
	// mapping its frontmatter and copying bundled files. (SK-IMP)
	ImportSkill(source, location, slug string, shared bool) (SkillImportResult, error)
}

// SkillImportResult is the tools-package view of an import outcome (mirrors
// skills.ImportResult, kept here so tools need not import the skills package).
type SkillImportResult struct {
	Slug     string
	Warnings []string
	Files    []string
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
		Description: "Create a reusable workspace skill: a named set of markdown instructions that agents can later load with use_skill. Provide a slug (kebab-case id), a name, a description, an optional whenToUse hint (when an agent should reach for it), an optional group (organisation label; skills sharing a group are folded together in the Skills UI), and the body (the full markdown instructions). The skill is available from the next turn. Returns the slug.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"slug":{"type":"string","description":"Kebab-case identifier, e.g. \"weekly-report\""},
				"name":{"type":"string","description":"Human-readable name"},
				"description":{"type":"string","description":"One-line summary shown in the catalog"},
				"whenToUse":{"type":"string","description":"Optional: when an agent should use this skill"},
				"group":{"type":"string","description":"Optional: organisation label; skills with the same group are folded together in the UI"},
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
		Group       string `json:"group"`
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
	if err := t.w.CreateSkill(in.Slug, in.Name, in.Description, in.WhenToUse, in.Group, in.Body, shared); err != nil {
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
		Description: "Edit an existing workspace skill in place by slug. Pass only the fields to change — name, description, whenToUse, group (organisation label), body (full markdown, replaces the old one), or shared. Omitted fields keep their current value. The slug (folder) is immutable; only workspace-tier skills can be edited. Prefer this over delete_skill + create_skill. Takes effect from the next turn. Returns the slug.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"slug":{"type":"string","description":"The skill slug to edit (see use_skill / the catalog)"},
				"name":{"type":"string","description":"New human-readable name"},
				"description":{"type":"string","description":"New one-line summary"},
				"whenToUse":{"type":"string","description":"New when-to-use hint"},
				"group":{"type":"string","description":"New organisation label (empty string clears it)"},
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
		Group       *string `json:"group"`
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
	if in.Name == nil && in.Description == nil && in.WhenToUse == nil && in.Group == nil && in.Body == nil && in.Shared == nil {
		return "", fmt.Errorf("nothing to update — provide at least one of: name, description, whenToUse, group, body, shared")
	}
	if err := t.w.UpdateSkill(in.Slug, in.Name, in.Description, in.WhenToUse, in.Group, in.Body, in.Shared); err != nil {
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

// ---- import_skill ----

// ImportSkillTool imports a Claude Code skill (from a local directory or a GitHub
// URL) into the workspace, mapping its frontmatter and copying bundled files so
// the rich CC skill ecosystem can be reused inside TionHarness. (SK-IMP)
type ImportSkillTool struct{ w SkillWriter }

// NewImportSkillTool constructs import_skill over a skill writer.
func NewImportSkillTool(w SkillWriter) ImportSkillTool { return ImportSkillTool{w: w} }

func (ImportSkillTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "import_skill",
		Description: "Import a Claude Code skill into this workspace. source \"local\" reads a skill " +
			"folder from disk (path = directory containing SKILL.md); source \"github\" fetches a " +
			"github.com folder URL (e.g. https://github.com/owner/repo/tree/main/skills/my-skill). " +
			"Frontmatter is mapped (allowed-tools→always_allow, paths→conditional, version/license/source " +
			"recorded); unsupported CC features (context:fork, hooks, slash-command args) are dropped with " +
			"warnings. Bundled files are copied. Returns the new slug + warnings.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"source":{"type":"string","enum":["local","github"],"description":"Where to import from (default local)"},
				"location":{"type":"string","description":"Local directory path (local) or github.com folder URL (github)"},
				"slug":{"type":"string","description":"Optional slug override; derived from the skill name when omitted"},
				"shared":{"type":"boolean","description":"Advertise to all agents (default false = restricted)"}
			},
			"required":["location"],
			"additionalProperties":false
		}`),
	}
}

func (t ImportSkillTool) Call(_ context.Context, input json.RawMessage) (string, error) {
	if t.w == nil {
		return "", fmt.Errorf("skill import is not available in this context")
	}
	var in struct {
		Source   string `json:"source"`
		Location string `json:"location"`
		Slug     string `json:"slug"`
		Shared   bool   `json:"shared"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "", argErr(err)
	}
	in.Location = strings.TrimSpace(in.Location)
	if in.Location == "" {
		return "", fmt.Errorf("location is required (a local directory path or a github.com URL)")
	}
	res, err := t.w.ImportSkill(in.Source, in.Location, strings.TrimSpace(in.Slug), in.Shared)
	if err != nil {
		return "", fmt.Errorf("import skill: %w", err)
	}
	out := map[string]any{"slug": res.Slug, "action": "imported"}
	if len(res.Files) > 0 {
		out["bundledFiles"] = res.Files
	}
	if len(res.Warnings) > 0 {
		out["warnings"] = res.Warnings
	}
	b, _ := json.Marshal(out)
	return string(b), nil
}

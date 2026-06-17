// Package skills implements a lightweight, file-based skill system: reusable
// instruction sets an agent can load on demand. Each skill is a directory
// containing a SKILL.md file (YAML-ish frontmatter + a markdown body). Skills
// are resolved from three tiers (global → workspace → project); a skill with the
// same slug in a higher tier overrides the lower one.
//
// Loading is deliberately LAZY: scanning parses only the frontmatter (name,
// description, when-to-use) so the per-turn catalog stays cheap. The full body
// is read from disk only when an agent invokes the use_skill tool — it never
// sits in the context window until it is actually needed.
package skills

// Source identifies which tier a skill was resolved from. Higher tiers override
// lower ones when slugs collide (project beats workspace beats global).
type Source string

const (
	// SourceGlobal is the cross-tool convention dir (~/.agents/skills).
	SourceGlobal Source = "global"
	// SourceWorkspace is this workspace's skills dir (<workspace>/skills).
	SourceWorkspace Source = "workspace"
	// SourceProject is the agent sandbox's project dir (<workDir>/.agents/skills).
	SourceProject Source = "project"
)

// Skill is one resolved skill. Only the frontmatter metadata is held in memory;
// the body lives on disk at Path and is read on demand (see Store.Body).
type Skill struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	WhenToUse   string   `json:"whenToUse,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Color       string   `json:"color,omitempty"`
	// AlwaysAllow lists tool-name patterns a skill expects to be auto-allowed.
	// Carried for parity / future enforcement; surfaced in the UI today.
	AlwaysAllow []string `json:"alwaysAllow,omitempty"`
	// RequiredSources lists source slugs the skill leans on (parity with the
	// craft convention; informational today).
	RequiredSources []string `json:"requiredSources,omitempty"`
	// Source is the tier this skill was resolved from.
	Source Source `json:"source"`
	// Path is the absolute path of the backing SKILL.md (not serialised; the
	// body is exposed via the detail endpoint instead).
	Path string `json:"-"`
}

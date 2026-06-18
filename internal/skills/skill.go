// Package skills implements a lightweight, file-based skill system: reusable
// instruction sets an agent can load on demand. Each skill is a directory
// containing a SKILL.md file (YAML-ish frontmatter + a markdown body). Skills
// are resolved from two tiers (global → workspace); a skill with the same slug
// in the workspace tier overrides the global one.
//
// Loading is deliberately LAZY: scanning parses only the frontmatter (name,
// description, when-to-use) so the per-turn catalog stays cheap. The full body
// is read from disk only when an agent invokes the use_skill tool — it never
// sits in the context window until it is actually needed.
package skills

// Source identifies which tier a skill was resolved from. The workspace tier
// overrides the global tier when slugs collide (workspace beats global).
type Source string

const (
	// SourceGlobal is SwarmGo's data-dir global skills dir (<DataDir>/skills).
	SourceGlobal Source = "global"
	// SourceWorkspace is this workspace's skills dir (<workspace>/skills).
	SourceWorkspace Source = "workspace"
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
	// Shared marks an "on-demand" skill: its summary is advertised to EVERY agent
	// and any agent may load it via use_skill, without explicit assignment. A
	// non-shared (restricted) skill is only visible/usable to agents it is
	// assigned to. Set from frontmatter `access: shared` (default restricted).
	Shared bool `json:"shared"`
	// AutoSummary controls whether this skill's one-line summary is AUTO-injected
	// into every agent's "# Available Skills" prompt block. Defaults to true. When
	// false, a shared skill is no longer advertised automatically (it stops
	// bloating every prompt) — it can still be assigned to an agent explicitly,
	// which always advertises it. Set from frontmatter `auto_summary: false`.
	AutoSummary bool `json:"autoSummary"`
	// AlwaysAllow lists tool-name patterns a skill expects to be auto-allowed.
	// Carried for parity / future enforcement; surfaced in the UI today.
	AlwaysAllow []string `json:"alwaysAllow,omitempty"`
	// RequiredSources lists source slugs the skill leans on (parity with the
	// craft convention; informational today).
	RequiredSources []string `json:"requiredSources,omitempty"`
	// SubSkills lists slugs of more detailed skills this one builds on. They are
	// advertised in a footer when the body is loaded via use_skill, so the model
	// can progressively load deeper instructions on demand (e.g. an overview skill
	// pointing at a detailed per-feature skill). Set from frontmatter `subskills:`.
	SubSkills []string `json:"subSkills,omitempty"`
	// Source is the tier this skill was resolved from.
	Source Source `json:"source"`
	// Path is the absolute path of the backing SKILL.md (not serialised; the
	// body is exposed via the detail endpoint instead).
	Path string `json:"-"`
}

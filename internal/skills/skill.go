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
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	WhenToUse   string `json:"whenToUse,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Color       string `json:"color,omitempty"`
	// Group is a free-form label used purely for ORGANISATION in the Skills UI:
	// skills sharing a Group are listed under a collapsible (fold in/out) header.
	// It has no effect on resolution, advertising, or loading — it is metadata for
	// humans. Empty means the skill is ungrouped. Set from frontmatter `group`.
	Group string `json:"group,omitempty"`
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
	// NameOnly marks a skill to advertise as SLUG ONLY in the "# Available Skills"
	// block: its one-line description + when-to-use are suppressed, leaving just the
	// backticked slug — the skill analogue of a tool's NameOnly (Claude Code
	// deferred-tool) tier. The skill stays LISTED (unlike auto_summary:false / paths,
	// which drop it from the block entirely), so the model still sees it exists and
	// can call skill_search to learn what it does before use_skill. Defaults to false
	// (full summary). Opt-in per skill via frontmatter `name_only: true` — NOT a
	// blanket default, because for skills the description is the main trigger signal.
	NameOnly bool `json:"nameOnly"`
	// Version/SourceURL/License are provenance metadata (SK-4) — important for
	// imported skills so their origin and currency are traceable. From frontmatter
	// `version` / `source_url` (alias `repo`/`homepage`) / `license`.
	Version   string `json:"version,omitempty"`
	SourceURL string `json:"sourceUrl,omitempty"`
	License   string `json:"license,omitempty"`
	// UserInvocable mirrors Claude Code's `user-invocable` (default true): a
	// background-knowledge skill sets it false. Informational in SwarmGo today
	// (skills load via use_skill, not slash commands); carried for import fidelity.
	UserInvocable bool `json:"userInvocable"`
	// AlwaysAllow lists tool-name patterns a skill expects to be auto-allowed.
	// Carried for parity / future enforcement; surfaced in the UI today.
	AlwaysAllow []string `json:"alwaysAllow,omitempty"`
	// RequiredSources lists source slugs the skill leans on (parity with the
	// craft convention; informational today).
	RequiredSources []string `json:"requiredSources,omitempty"`
	// Paths marks a skill CONDITIONAL: when non-empty, the skill is NOT advertised
	// in the per-turn catalog automatically (so it never bloats the prompt), even if
	// shared — it is found on demand via the skill_search tool or reached by explicit
	// assignment. The patterns mirror Claude Code's `paths:` frontmatter (the file
	// globs the skill is relevant to). Set from frontmatter `paths:`. (SK-2)
	Paths []string `json:"paths,omitempty"`
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

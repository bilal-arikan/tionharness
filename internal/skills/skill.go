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

import "strings"

// Visibility tiers describe how much of a skill rides in the per-turn "# Available
// Skills" catalog block — the skill analogue of a tool's visibility tiers (see
// tools.Visibility*). Every skill resolves to exactly one tier, derived from its
// frontmatter flags (see skillVisibility); the Skills screen sets it via one
// 4-way selector that maps back onto those flags.
const (
	// VisibilityFull: advertised with slug + description + when-to-use (richest).
	VisibilityFull = "full"
	// VisibilitySummary: advertised with slug + description only (when-to-use
	// suppressed) — a leaner middle tier.
	VisibilitySummary = "summary"
	// VisibilityNameOnly: advertised by slug ALONE (description + when-to-use
	// suppressed); still listed so the model can skill_search it before use_skill.
	VisibilityNameOnly = "name-only"
	// VisibilityHidden: folded OUT of the catalog entirely (not even named);
	// reachable only via explicit assignment or skill_search.
	VisibilityHidden = "hidden"
)

// KindCoordinatorWorkflow marks a skill as a saved coordinator recipe (M5): a
// reusable orchestration pattern rather than plain instructions. Match it
// case-insensitively via IsCoordinatorWorkflow.
const KindCoordinatorWorkflow = "coordinator-workflow"

// PatternValues are the orchestration strategies a coordinator-workflow recipe
// may encode (see _Docs/47 §4). "custom" is the escape hatch for a recipe whose
// body defines its own combination.
var PatternValues = []string{
	"fanout",          // Fanout-And-Synthesize
	"adversarial",     // Adversarial Verification
	"loop",            // Loop Until Done
	"classify",        // Classify-And-Act
	"generate-filter", // Generate-And-Filter
	"tournament",      // Tournament
	"custom",          // recipe-defined combination
}

// KnownPattern reports whether p is one of PatternValues (case-insensitive).
func KnownPattern(p string) bool {
	for _, v := range PatternValues {
		if strings.EqualFold(strings.TrimSpace(p), v) {
			return true
		}
	}
	return false
}

// IsCoordinatorWorkflow reports whether this skill is a saved coordinator recipe.
func (s Skill) IsCoordinatorWorkflow() bool {
	return strings.EqualFold(strings.TrimSpace(s.Kind), KindCoordinatorWorkflow)
}

// Source identifies which tier a skill was resolved from. The workspace tier
// overrides the global tier when slugs collide (workspace beats global).
type Source string

const (
	// SourceGlobal is TionSwarm's data-dir global skills dir (<DataDir>/skills).
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
	// SummaryOnly marks a skill to advertise as slug + description ONLY (its
	// when-to-use is suppressed) — the middle "summary" visibility tier between
	// full and name-only. Defaults to false. Set from frontmatter `summary_only:
	// true`. Ignored when NameOnly is set (name-only is the stronger suppression).
	SummaryOnly bool `json:"summaryOnly"`
	// Visibility is the DERIVED 4-way tier (full | summary | name-only | hidden)
	// computed from AutoSummary/NameOnly/SummaryOnly (see skillVisibility). It is
	// not stored directly — it is the single value the Skills screen reads and
	// writes, mapping back onto the underlying flags. Serialised for the UI.
	Visibility string `json:"visibility"`
	// Version/SourceURL/License are provenance metadata (SK-4) — important for
	// imported skills so their origin and currency are traceable. From frontmatter
	// `version` / `source_url` (alias `repo`/`homepage`) / `license`.
	Version   string `json:"version,omitempty"`
	SourceURL string `json:"sourceUrl,omitempty"`
	License   string `json:"license,omitempty"`
	// UserInvocable mirrors Claude Code's `user-invocable` (default true): a
	// background-knowledge skill sets it false. Informational in TionSwarm today
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
	// Kind classifies a non-standard skill. Empty is a normal instructional skill;
	// "coordinator-workflow" marks a saved coordinator recipe (M5) — a reusable
	// orchestration pattern selectable in the coordinator composer. Set from
	// frontmatter `kind`. UI lists coordinator-workflow skills in a separate picker.
	Kind string `json:"kind,omitempty"`
	// Pattern is the orchestration strategy a coordinator-workflow recipe encodes
	// (one of PatternValues). Only meaningful when Kind=="coordinator-workflow".
	// Set from frontmatter `pattern`. Validated at apply time (KnownPattern).
	Pattern string `json:"pattern,omitempty"`
	// WorkerTargets are the DEFAULT fan-out targets (agent names or profiles like
	// explore/coder/reviewer) a coordinator-workflow suggests. Advisory — the
	// coordinator may override them. Set from frontmatter `worker_targets`.
	WorkerTargets []string `json:"workerTargets,omitempty"`
	// StopCondition is a human-readable done-criteria for loop-style recipes
	// (e.g. "no new findings in 2 rounds"). Injected into the coordinator prompt.
	// Set from frontmatter `stop_condition`.
	StopCondition string `json:"stopCondition,omitempty"`
	// MaxTurns optionally overrides CoordinatorMaxTurns for this recipe (0 = keep
	// the workspace default). Set from frontmatter `max_turns`.
	MaxTurns int `json:"maxTurns,omitempty"`
	// Source is the tier this skill was resolved from.
	Source Source `json:"source"`
	// ModifiedAt is the SKILL.md file's last-modified time (Unix seconds), surfaced
	// so the Skills screen can show a "last edited" date and sort skills within a
	// group newest-first. 0 when the file could not be stat'd. Set in scanDir.
	ModifiedAt int64 `json:"modifiedAt,omitempty"`
	// Path is the absolute path of the backing SKILL.md (not serialised; the
	// body is exposed via the detail endpoint instead).
	Path string `json:"-"`
}

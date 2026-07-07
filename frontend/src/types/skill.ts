// Skills — reusable agent instruction sets resolved from three tiers
// (global / workspace / project). The catalog carries frontmatter only; the
// markdown body is fetched on demand via the detail endpoint.

import type { ToolVisibility } from './workspace'

export type SkillSource = 'global' | 'workspace'

export interface Skill {
  slug: string
  name: string
  description: string
  whenToUse?: string
  icon?: string
  color?: string
  // Free-form organisation label. Skills sharing a group are listed under a
  // collapsible header in the Skills UI. Purely cosmetic — no effect on
  // resolution or loading. Empty/undefined means ungrouped.
  group?: string
  alwaysAllow?: string[]
  requiredSources?: string[]
  // Slugs of more detailed skills this one builds on (progressive disclosure).
  // Advertised in a footer when the body loads via use_skill.
  subSkills?: string[]
  // When true the skill is "on-demand": advertised to every agent and usable
  // without assignment. Otherwise it is restricted to agents it is assigned to.
  shared?: boolean
  // When false, the skill's one-line summary is NOT auto-injected into every
  // agent's prompt (it stops bloating every session). Defaults to true. A skill
  // assigned explicitly to an agent is always advertised regardless.
  autoSummary?: boolean
  // When true the skill is advertised as SLUG ONLY in the Available Skills block
  // (description + when-to-use suppressed) — the skill analogue of a tool's
  // NameOnly tier. It stays listed (the model can skill_search it); defaults to
  // false. Unlike autoSummary:false / paths, the skill is NOT dropped entirely.
  nameOnly?: boolean
  // When true the skill is advertised as slug + description only (when-to-use
  // suppressed) — the middle "summary" visibility tier between full and
  // name-only. Defaults to false. Ignored when nameOnly is set.
  summaryOnly?: boolean
  // Derived 4-way visibility tier (full | summary | name-only | hidden) computed
  // from autoSummary/nameOnly/summaryOnly on the backend. The single value the
  // Skills screen's tier selector reads and writes; mirrors a tool's visibility.
  visibility?: ToolVisibility
  source: SkillSource
  // SKILL.md last-modified time (Unix seconds). Surfaced in the Skills screen as
  // a "last edited" label; skills are sorted within each group newest-first by it.
  modifiedAt?: number
}

export interface SkillDetail extends Skill {
  // Full markdown instructions (frontmatter stripped), loaded lazily.
  body: string
  // Folder containing the skill's SKILL.md (for copy-path / reveal). May be "".
  dir: string
}

// SkillInput is the editable payload sent when creating or updating a skill.
// Create may also include an optional `slug`; on update the slug is immutable.
export interface SkillInput {
  name: string
  description: string
  whenToUse: string
  icon: string
  color: string
  group: string
  shared: boolean
  body: string
}

// Skills — reusable agent instruction sets resolved from three tiers
// (global / workspace / project). The catalog carries frontmatter only; the
// markdown body is fetched on demand via the detail endpoint.

export type SkillSource = 'global' | 'workspace' | 'project'

export interface Skill {
  slug: string
  name: string
  description: string
  whenToUse?: string
  icon?: string
  color?: string
  alwaysAllow?: string[]
  requiredSources?: string[]
  // When true the skill is "on-demand": advertised to every agent and usable
  // without assignment. Otherwise it is restricted to agents it is assigned to.
  shared?: boolean
  source: SkillSource
}

export interface SkillDetail extends Skill {
  // Full markdown instructions (frontmatter stripped), loaded lazily.
  body: string
}

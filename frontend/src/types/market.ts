// Marketplace — shareable "packs" (skill / agent / provider / flow) resolved
// from three tiers (bundled / global / workspace). The catalog carries the
// manifest only; the kind-specific payload is fetched on demand via the detail
// endpoint and used at install time. See _Docs/21-MARKET.md.

export type PackKind = 'skill' | 'agent' | 'provider' | 'flow'
export type PackSource = 'bundled' | 'global' | 'workspace'

export interface SkillPayload {
  slug: string
  body: string
}

export interface PackPayload {
  skill?: SkillPayload
  // agent / provider / flow payloads land in later slices.
}

export interface Pack {
  schema: string
  id: string
  kind: PackKind
  name: string
  description: string
  version?: string
  author?: string
  icon?: string
  color?: string
  tags?: string[]
  createdAt?: number
  // Populated only on the detail endpoint (GET /api/market/{id}).
  payload?: PackPayload
  source?: PackSource
}

export interface InstallResult {
  kind: PackKind
  ref: string
  message: string
}

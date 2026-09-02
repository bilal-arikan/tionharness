// Curator report (Rota F3), mirroring internal/db/store_curator.go.

export type CuratorActionKind = 'archive' | 'suggest'
export type CuratorEntity = 'automation' | 'schedule' | 'hook' | 'recipe'
// exhausted | expired | one_shot_done | never_fired | unfired_watcher | ghost_phase
export type CuratorReason = string

export interface CuratorAction {
  kind: CuratorActionKind
  entity: CuratorEntity
  id: string
  name?: string
  reason: CuratorReason
  detail?: string
  applied: boolean
  evidence?: string
}

export interface CuratorReport {
  at: number
  trigger: 'manual' | 'weekly' | string
  idle: boolean
  archived: number
  suggestions: number
  actions: CuratorAction[]
}

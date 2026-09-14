// Labels, tones and formatting for the Goals screen (pure, testable).
import type { BadgeTone } from '@/shared/components'
import type {
  Goal,
  GoalAuthor,
  GoalDirection,
  GoalMetricDef,
  GoalPolicyMode,
  GoalStatus,
} from '@/types/goal'

export const STATUS_LABEL: Record<GoalStatus, string> = {
  draft: 'Taslak',
  active: 'Etkin',
  paused: 'Duraklatıldı',
  archived: 'Arşiv',
}

export const STATUS_TONE: Record<GoalStatus, BadgeTone> = {
  draft: 'warning',
  active: 'success',
  paused: 'muted',
  archived: 'muted',
}

export const DIRECTION_LABEL: Record<GoalDirection, string> = {
  min: 'düşsün',
  max: 'artsın',
}

export const MODE_LABEL: Record<GoalPolicyMode, string> = {
  propose: 'Öner',
  off: 'Yalnız ölç',
}

export const MODE_HINT: Record<GoalPolicyMode, string> = {
  propose: 'Evrim geçişi bulgu üretir; her değişikliği sen onaylarsın.',
  off: 'Hedef yalnız ölçülür, öneri üretilmez.',
}

export const SCOPE_LABEL = {
  recipes: 'Reçeteler',
  agents: 'Ajanlar',
  automations: 'Otomasyonlar',
  tags: 'Etiketler',
} as const

// authorLabel names who made a change.
export function authorLabel(by: GoalAuthor | undefined): string {
  if (!by) return '—'
  if (by === 'user') return 'Sen'
  if (by === 'agent:goal-writer') return 'Hedef yazıcı'
  if (by === 'seed') return 'Başlangıç hedefi'
  if (by.startsWith('agent:')) return by.slice('agent:'.length)
  return by
}

// Status transitions the screen offers from each state.
export function nextStatuses(s: GoalStatus): GoalStatus[] {
  switch (s) {
    case 'draft':
      return ['active', 'archived']
    case 'active':
      return ['paused', 'archived']
    case 'paused':
      return ['active', 'archived']
    case 'archived':
      return ['draft']
  }
}

export const STATUS_ACTION_LABEL: Record<GoalStatus, string> = {
  draft: 'Taslağa al',
  active: 'Etkinleştir',
  paused: 'Duraklat',
  archived: 'Arşivle',
}

// metricLabel resolves a catalog key to its label (falls back to the key).
export function metricLabel(key: string, catalog: GoalMetricDef[] | undefined): string {
  return catalog?.find((m) => m.key === key)?.label ?? key
}

// formatMetricValue renders a bound/target with its unit.
export function formatMetricValue(v: number | null | undefined, unit: string | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return '—'
  switch (unit) {
    case 'usd':
      return `$${v.toFixed(v < 1 ? 3 : 2)}`
    case 'ratio':
      return `${Math.round(v * 1000) / 10}%`
    case 'sec':
      return v >= 3600
        ? `${(v / 3600).toFixed(1)} sa`
        : v >= 60
          ? `${Math.round(v / 60)} dk`
          : `${v} sn`
    case 'tokens':
      return v >= 1000 ? `${(v / 1000).toFixed(1)}k tok` : `${v} tok`
    default:
      return String(v)
  }
}

// scopeSummary is the one-line scope chip text: "tüm workspace" or counts.
export function scopeSummary(g: Goal): string {
  const parts: string[] = []
  const n = (l?: string[]) => l?.length ?? 0
  if (n(g.scope.recipes)) parts.push(`${n(g.scope.recipes)} reçete`)
  if (n(g.scope.agents)) parts.push(`${n(g.scope.agents)} ajan`)
  if (n(g.scope.automations)) parts.push(`${n(g.scope.automations)} otomasyon`)
  if (n(g.scope.tags)) parts.push(`${n(g.scope.tags)} etiket`)
  return parts.length ? parts.join(' · ') : 'tüm workspace'
}

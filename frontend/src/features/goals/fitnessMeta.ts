// Pure helpers for the fitness block: window presets, change labels, and the
// verdict text for a metric against its direction/target.
import type { GoalFitness, MetricValue, SnapshotChange } from '@/types/evolution'

export const WINDOWS: { key: string; label: string; days: number | null }[] = [
  { key: '7d', label: '7 gün', days: 7 },
  { key: '30d', label: '30 gün', days: 30 },
  { key: '90d', label: '90 gün', days: 90 },
  { key: 'all', label: 'Tümü', days: null },
]

// sinceFor turns a window preset into the unix-seconds `since` the API takes
// (0 = everything).
export function sinceFor(key: string, nowMs = Date.now()): number {
  const w = WINDOWS.find((x) => x.key === key) ?? WINDOWS[1]
  if (w.days === null) return 0
  return Math.floor(nowMs / 1000) - w.days * 86400
}

export const SURFACE_LABEL: Record<string, string> = {
  agent: 'Ajan',
  tools: 'Araçlar',
  recipe: 'Reçete',
  automation: 'Otomasyon',
  schedule: 'Zamanlama',
  prompt: 'Prompt',
  settings: 'Ayarlar',
  model: 'Model',
}

// changeLabel renders one snapshot diff entry as a short line.
export function changeLabel(c: SnapshotChange): string {
  const head = [SURFACE_LABEL[c.surface] ?? c.surface, c.entity].filter(Boolean).join(' ')
  if (c.field === 'added') return `${head} eklendi${c.after ? ` (${c.after})` : ''}`
  if (c.field === 'removed') return `${head} kaldırıldı${c.before ? ` (${c.before})` : ''}`
  const val = (v?: string) =>
    v === undefined || v === '' ? '—' : v.length > 24 ? v.slice(0, 22) + '…' : v
  return `${head} · ${c.field}: ${val(c.before)} → ${val(c.after)}`
}

// primaryVerdict says whether the primary value is on target / trending well.
export function primaryVerdict(f: GoalFitness): 'ok' | 'off' | 'none' {
  if (f.primary.value === null || f.primary.value === undefined) return 'none'
  if (f.onTarget === true) return 'ok'
  if (f.onTarget === false) return 'off'
  return 'none'
}

// deltaLabel compares a value with the previous snapshot's, respecting direction.
export function deltaLabel(
  cur: MetricValue,
  prev: MetricValue | undefined,
  direction: 'min' | 'max',
): { text: string; good: boolean | null } {
  if (!prev || cur.value === null || prev.value === null || prev.value === undefined) {
    return { text: '', good: null }
  }
  const d = cur.value - prev.value
  if (d === 0) return { text: '±0', good: null }
  const pct =
    prev.value !== 0 ? ` (${d > 0 ? '+' : ''}${Math.round((d / Math.abs(prev.value)) * 100)}%)` : ''
  const good = direction === 'min' ? d < 0 : d > 0
  return { text: `${d > 0 ? '▲' : '▼'}${pct}`, good }
}

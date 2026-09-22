// Pure helpers for the fitness block: window presets, change labels, and the
// verdict text for a metric against its direction/target.
import type { GoalFitness, MetricValue, SnapshotChange, SnapshotFitness } from '@/types/evolution'

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

const SURFACE_LABEL: Record<string, string> = {
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

interface Delta {
  text: string
  good: boolean | null
}

// deltaLabel compares a value with the previous snapshot's, respecting direction.
export function deltaLabel(
  cur: MetricValue,
  prev: MetricValue | undefined,
  direction: 'min' | 'max',
): Delta {
  return deltaBetween(cur.value, prev ? prev.value : null, direction)
}

function deltaBetween(
  cur: number | null | undefined,
  prev: number | null | undefined,
  direction: 'min' | 'max',
): Delta {
  if (cur === null || cur === undefined || prev === null || prev === undefined) {
    return { text: '', good: null }
  }
  const d = cur - prev
  if (d === 0) return { text: '±0', good: null }
  const pct = prev !== 0 ? ` (${d > 0 ? '+' : ''}${Math.round((d / Math.abs(prev)) * 100)}%)` : ''
  const good = direction === 'min' ? d < 0 : d > 0
  return { text: `${d > 0 ? '▲' : '▼'}${pct}`, good }
}

// Which per-session statistic the version breakdown shows. Median is the
// default: a bucket's mean is easily dominated by a few heavy sessions.
export type StatKind = 'median' | 'trimmed' | 'mean'

export const STATS: { key: StatKind; label: string }[] = [
  { key: 'median', label: 'Medyan' },
  { key: 'trimmed', label: 'Kırpılmış ort.' },
  { key: 'mean', label: 'Ortalama' },
]

// statValue picks the chosen statistic of a per-session metric; metrics
// without a distribution only have their plain value.
export function statValue(m: MetricValue, stat: StatKind): number | null {
  if (m.value === null || m.value === undefined) return null
  if (!m.dist) return m.value
  if (stat === 'median') return m.dist.median
  if (stat === 'trimmed') return m.dist.trimmedMean
  return m.value
}

// snapshotDelta is the version-vs-previous-version badge. When either bucket
// fails the insufficient-data gate (too few sessions, or dominated by its top
// three) no delta is drawn: the raw values stay visible, the badge says
// "yetersiz veri" and lists why.
export function snapshotDelta(
  cur: SnapshotFitness,
  prev: SnapshotFitness | undefined,
  direction: 'min' | 'max',
  stat: StatKind,
): Delta & { gated: boolean; reasons: string[] } {
  if (!prev) return { text: '', good: null, gated: false, reasons: [] }
  const gatedBuckets = [cur, prev].filter((b) => b.stats?.insufficient)
  if (gatedBuckets.length > 0) {
    const reasons = gatedBuckets.flatMap((b) =>
      (b.stats.reasons ?? []).map((r) => `${b === cur ? 'bu sürüm' : 'önceki'}: ${r}`),
    )
    return { text: 'yetersiz veri', good: null, gated: true, reasons }
  }
  return {
    ...deltaBetween(statValue(cur.primary, stat), statValue(prev.primary, stat), direction),
    gated: false,
    reasons: [],
  }
}

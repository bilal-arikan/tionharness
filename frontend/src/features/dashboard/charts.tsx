import type { DaySeriesPoint, NamedCount } from '@/types'

// Hand-rolled SVG/CSS charts, matching how features/sessions/viz already draws
// its Sankey and Gantt. A charting library would add a large dependency for
// three shapes that are a few dozen lines each, and these inherit the theme's
// CSS variables for free.
//
// Shared rule: every chart states its own empty case in words. A blank chart
// area is ambiguous — "no data" and "failed to load" look identical — and this
// screen's job is to be trustworthy at a glance.

const EMPTY = 'text-xs text-[var(--color-text-dim)]'

// compact renders large counts as 12k / 3.4M so an axis label stays short.
export function compact(n: number): string {
  if (n < 1000) return String(n)
  if (n < 1_000_000) return `${Math.round(n / 100) / 10}k`
  return `${Math.round(n / 100_000) / 10}M`
}

// dayLabel is the short "04.08" form used on the x-axis.
function dayLabel(day: string): string {
  const [, m, d] = day.split('-')
  return `${d}.${m}`
}

interface SparkProps {
  points: DaySeriesPoint[]
  color?: string
  // Height of the plot area in px.
  height?: number
  label: string
}

// DayBars is the daily trend: one bar per day, with the peak value called out.
// Bars (not a line) because the series is a COUNT per bucket — a line would
// imply values between the days that do not exist.
export function DayBars({ points, color = 'var(--color-accent)', height = 64, label }: SparkProps) {
  if (points.length === 0) return <p className={EMPTY}>{label}: veri yok</p>
  const max = Math.max(...points.map((p) => p.value))
  const total = points.reduce((a, p) => a + p.value, 0)

  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-xs font-medium">{label}</span>
        <span className="text-xs text-[var(--color-text-dim)]">
          toplam {compact(total)} · tepe {compact(max)}
        </span>
      </div>
      {max === 0 ? (
        // An all-zero window is a real answer, not a missing one — say it rather
        // than drawing a row of invisible bars.
        <p className={EMPTY}>bu aralıkta hiç hareket yok</p>
      ) : (
        <div className="flex items-end gap-[2px]" style={{ height }}>
          {points.map((p) => (
            <div
              key={p.day}
              title={`${p.day}: ${p.value}`}
              className="min-w-[3px] flex-1 rounded-sm transition-opacity hover:opacity-70"
              style={{
                // A non-zero day always keeps 2px so it never reads as an empty
                // day just because it is small next to a spike.
                height: p.value === 0 ? 1 : Math.max(2, (p.value / max) * height),
                background: p.value === 0 ? 'var(--color-border)' : color,
              }}
            />
          ))}
        </div>
      )}
      <div className="mt-1 flex justify-between text-[10px] text-[var(--color-text-dim)]">
        <span>{dayLabel(points[0].day)}</span>
        <span>{dayLabel(points[points.length - 1].day)}</span>
      </div>
    </div>
  )
}

// Palette for categorical charts. Fixed order → a category keeps its colour
// between refreshes, which is what makes the chart readable at a glance.
const PALETTE = [
  'var(--color-accent)',
  '#22c55e',
  '#eab308',
  '#ef4444',
  '#a855f7',
  '#06b6d4',
  '#f97316',
  '#64748b',
]

// colorFor gives known statuses their semantic colour and everything else a
// stable slot from the palette.
function colorFor(name: string, index: number): string {
  switch (name) {
    case 'success':
      return '#22c55e'
    case 'failure':
      return '#ef4444'
    case 'waiting':
      return '#eab308'
    case 'running':
      return 'var(--color-accent)'
    case 'done':
      return '#22c55e'
    case 'failed':
      return '#ef4444'
    case 'in_progress':
      return 'var(--color-accent)'
    case 'review':
      return '#a855f7'
    default:
      return PALETTE[index % PALETTE.length]
  }
}

// StackedBar is the one-line composition chart used for board columns and run
// statuses: proportions at a glance, exact counts in the legend below.
export function StackedBar({ items, label }: { items: NamedCount[]; label: string }) {
  const total = items.reduce((a, i) => a + i.count, 0)
  if (total === 0) return <p className={EMPTY}>{label}: henüz kayıt yok</p>

  return (
    <div>
      <div className="mb-1 flex items-baseline justify-between">
        <span className="text-xs font-medium">{label}</span>
        <span className="text-xs text-[var(--color-text-dim)]">{total}</span>
      </div>
      <div className="flex h-3 overflow-hidden rounded-full">
        {items.map((i, idx) => (
          <div
            key={i.name}
            title={`${i.name}: ${i.count} (%${Math.round((i.count / total) * 100)})`}
            style={{ width: `${(i.count / total) * 100}%`, background: colorFor(i.name, idx) }}
          />
        ))}
      </div>
      <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
        {items.map((i, idx) => (
          <span key={i.name} className="flex items-center gap-1 text-[11px]">
            <span
              className="inline-block h-2 w-2 shrink-0 rounded-full"
              style={{ background: colorFor(i.name, idx) }}
            />
            <span className="text-[var(--color-text-dim)]">{i.name}</span>
            <span className="font-medium">{i.count}</span>
          </span>
        ))}
      </div>
    </div>
  )
}

// RankBars is the horizontal ranking used for "which agents are busiest".
export function RankBars({ items, label }: { items: NamedCount[]; label: string }) {
  if (items.length === 0) return <p className={EMPTY}>{label}: henüz kayıt yok</p>
  const max = Math.max(...items.map((i) => i.count))

  return (
    <div>
      <div className="mb-1.5 text-xs font-medium">{label}</div>
      <div className="flex flex-col gap-1.5">
        {items.map((i) => (
          <div key={i.name} className="flex items-center gap-2">
            <span
              className="w-28 shrink-0 truncate text-[11px] text-[var(--color-text-dim)]"
              title={i.name}
            >
              {i.name}
            </span>
            <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-[var(--color-surface-2)]">
              <div
                className="h-full rounded-full"
                style={{ width: `${(i.count / max) * 100}%`, background: 'var(--color-accent)' }}
              />
            </div>
            <span className="w-8 shrink-0 text-right text-[11px] font-medium">{i.count}</span>
          </div>
        ))}
      </div>
    </div>
  )
}

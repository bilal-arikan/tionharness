import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'
import { buildConcurrencyTimeline, type TimelineModel } from './flowVizData'
import { formatTime } from '@/shared/lib/intl'

// Layout constants (SVG user units; the chart scales responsively via viewBox).
const W = 620
const GUTTER = 76 // left column for lane labels
const RIGHT = 10
const TOP = 6
const LANE_H = 22
const BAR_H = 12
const AXIS_H = 18

function clock(ms: number): string {
  try {
    return formatTime(new Date(ms), { hour12: false })
  } catch {
    return ''
  }
}

function fmtSpan(ms: number): string {
  if (ms >= 60_000) return `${(ms / 60_000).toFixed(1)} dk`
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)} s`
  return `${ms} ms`
}

// ConcurrencyTimeline draws a dependency-free SVG Gantt of a session's timed
// events (llm calls, or turns as a fallback), one lane per agent. Overlapping
// bars across lanes reveal genuine concurrency (a coordinator running workers);
// a single-agent run reads as a serial cadence. Self-contained: it derives its
// model from the raw debug events.
export function ConcurrencyTimeline({
  events,
  agentNames,
}: {
  events: SessionDebugEvent[]
  agentNames: Record<string, string>
}) {
  const model = useMemo(() => buildConcurrencyTimeline(events, agentNames), [events, agentNames])
  if (!model) {
    return (
      <p className="py-2 text-[11px] text-[var(--color-text-dim)]">
        Zaman çizelgesi için yeterli olay yok.
      </p>
    )
  }
  return <TimelineSVG model={model} />
}

function TimelineSVG({ model }: { model: TimelineModel }) {
  const { lanes, bars, minMs, maxMs, hasOverlap } = model
  const span = maxMs - minMs || 1
  const plotW = W - GUTTER - RIGHT
  const scale = (ms: number) => GUTTER + ((ms - minMs) / span) * plotW
  const laneIndex = Object.fromEntries(lanes.map((l, i) => [l.id, i]))
  const height = TOP + lanes.length * LANE_H + AXIS_H

  // Three evenly-spaced axis ticks (start / mid / end).
  const ticks = [0, 0.5, 1].map((f) => ({
    x: GUTTER + f * plotW,
    label: clock(minMs + f * span),
  }))

  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-2">
      <div className="mb-1 flex items-center justify-between text-[10px] text-[var(--color-text-dim)]">
        <span>
          {bars.length} olay · {lanes.length} ajan · {fmtSpan(span)}
        </span>
        <span className={hasOverlap ? 'text-[var(--color-accent)]' : ''}>
          {hasOverlap ? 'eşzamanlı' : 'seri'}
        </span>
      </div>
      <svg
        viewBox={`0 0 ${W} ${height}`}
        width="100%"
        height={height}
        preserveAspectRatio="xMidYMid meet"
      >
        {/* Lane rows: label + baseline */}
        {lanes.map((lane, i) => {
          const y = TOP + i * LANE_H
          return (
            <g key={lane.id}>
              <line
                x1={GUTTER}
                y1={y + LANE_H / 2}
                x2={W - RIGHT}
                y2={y + LANE_H / 2}
                stroke="var(--color-border)"
                strokeWidth={0.5}
                strokeDasharray="2 3"
              />
              <text
                x={GUTTER - 6}
                y={y + LANE_H / 2}
                textAnchor="end"
                dominantBaseline="middle"
                fontSize={10}
                fill="var(--color-text-dim)"
              >
                {lane.label.length > 11 ? `${lane.label.slice(0, 10)}…` : lane.label}
              </text>
            </g>
          )
        })}

        {/* Bars */}
        {bars.map((b, i) => {
          const x = scale(b.startMs)
          const w = Math.max(2, scale(b.endMs) - x)
          const y = TOP + (laneIndex[b.laneId] ?? 0) * LANE_H + (LANE_H - BAR_H) / 2
          return (
            <rect
              key={i}
              x={x}
              y={y}
              width={w}
              height={BAR_H}
              rx={2}
              fill={b.err ? 'var(--color-danger)' : 'var(--color-accent)'}
              fillOpacity={0.85}
            >
              <title>{`${clock(b.startMs)} → ${clock(b.endMs)}\n${b.detail}`}</title>
            </rect>
          )
        })}

        {/* Axis ticks */}
        {ticks.map((t, i) => (
          <g key={i}>
            <line
              x1={t.x}
              y1={TOP}
              x2={t.x}
              y2={height - AXIS_H}
              stroke="var(--color-border)"
              strokeWidth={0.5}
            />
            <text
              x={i === 0 ? t.x : i === ticks.length - 1 ? t.x : t.x}
              y={height - 5}
              textAnchor={i === 0 ? 'start' : i === ticks.length - 1 ? 'end' : 'middle'}
              fontSize={9}
              fill="var(--color-text-dim)"
            >
              {t.label}
            </text>
          </g>
        ))}
      </svg>
    </div>
  )
}

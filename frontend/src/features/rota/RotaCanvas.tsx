// Rota F0 canvas: an SVG git-graph over the pure layout (rotaLayout.ts).
// Deliberately plain SVG rather than React Flow for this projection-only
// phase — the layout is a fixed time × lane grid with no drag/pan needs, and
// SVG keeps it deterministic and cheap to redraw on every stream event.
// Interaction: click selects (→ side panel), double-click opens the entity,
// ↑/↓ move the selection across lanes (Harita contract).
import { useMemo } from 'react'
import type { RotaBar, RotaEdge, RotaLayout, RotaMark, RotaRow } from './rotaLayout'
import { laneOriginGlyph } from './rotaLabels'
import { buildTimeScale, formatGapSpan } from './rotaTimeScale'
import { formatWait } from './rotaWaits'

// What the side panel can project from either canvas: a session / flow run
// bar, a whole trajectory (phase header click), an automation ghost.
export interface RotaSelection {
  kind: 'session' | 'flowrun' | 'trajectory' | 'automation'
  id: string
}

interface Props {
  layout: RotaLayout
  width: number // available pixel width (the canvas fills it; wider content scrolls)
  selected: RotaSelection | null
  onSelect: (sel: RotaSelection | null) => void
  onOpenSession?: (sessionId: string) => void
  onOpenFlowRun?: (flowId: string) => void
  // Zoom into a lane's trajectory (the ◈ glyph).
  onOpenTrajectory?: (trajectoryId: string) => void
  // Collapse stretches of the past window where no lane did anything.
  collapseGaps?: boolean
  // Spend time on a log scale, so long sessions stop eating the panel while
  // short ones stay readable (rotaTimeScale.ts).
  normalizeBars?: boolean
  // How many times wider than the panel the time axis is drawn (rotaZoom.ts).
  // 1 fits the panel; above that the scroll container pans the canvas.
  zoom?: number
}

// Row pitch and label column are deliberately tight: the canvas is a density
// view, and every pixel of padding is one fewer lane on screen.
const ROW_H = 22
/** Lane label column. Exported because the zoom anchor has to know which part
 *  of the canvas does not scale. */
export const ROTA_LABEL_W = 180
const LABEL_W = ROTA_LABEL_W
const FUTURE_W = 140
const TOP_H = 20
const PAD_R = 8
/** Floor for a wait segment: a wait long enough to draw stays visible even when
 *  the window spans days and its true width would round to zero. */
const WAIT_MIN_PX = 3
/** Floor for a session bar, so an instant session is still clickable. */
const MIN_BAR_PX = 3

const EDGE_COLOR: Record<RotaEdge['kind'], string> = {
  spawned: '#f97316',
  reported: '#a855f7',
  forked_from: '#a855f7',
}

function barFill(b: RotaBar): string {
  if (b.live) return 'var(--color-accent)'
  switch (b.state) {
    case 'completed':
    case 'success':
    case 'done':
      return '#10b981'
    case 'failed':
    case 'failure':
    case 'killed':
    case 'timeout':
      return '#ef4444'
    case 'archived':
      return 'var(--color-border)'
    default:
      return 'var(--color-text-dim)'
  }
}

function markGlyph(m: RotaMark): string {
  switch (m.kind) {
    case 'fired':
      return '⚡'
    case 'skipped':
      return '↷'
    case 'stall':
      return '✕'
  }
}

export function RotaCanvas({
  layout,
  width,
  selected,
  onSelect,
  onOpenSession,
  onOpenFlowRun,
  onOpenTrajectory,
  collapseGaps = true,
  normalizeBars = true,
  zoom = 1,
}: Props) {
  const { rows, bars, edges, marks, future, t0, now, t1 } = layout
  // Past window fills what is left after the label column and the future
  // strip; never below a minimum so a very long history still scrolls.
  // Zoom stretches the past axis only; the label column and the future strip
  // keep their pixel widths so labels stay put and readable at every level.
  const pastW = Math.max(240, width - LABEL_W - FUTURE_W - PAD_R) * zoom
  const futureSpan = Math.max(1, t1 - now)
  // The past is piecewise (dead air collapsed to a sliver when asked for);
  // the future strip stays linear.
  const scale = useMemo(
    () =>
      buildTimeScale(layout, {
        x0: LABEL_W,
        width: pastW,
        collapse: collapseGaps,
        logDuration: normalizeBars,
      }),
    [layout, pastW, collapseGaps, normalizeBars],
  )
  const x = (t: number) =>
    t <= now ? scale.x(t) : LABEL_W + pastW + ((t - now) / futureSpan) * FUTURE_W
  const xNow = LABEL_W + pastW
  const height = TOP_H + Math.max(1, rows.length) * ROW_H + 6
  const svgW = LABEL_W + pastW + FUTURE_W + PAD_R
  const rowY = useMemo(
    () => new Map(rows.map((r) => [r.id, TOP_H + r.y * ROW_H + ROW_H / 2])),
    [rows],
  )

  const isSel = (kind: RotaSelection['kind'], id: string) =>
    selected?.kind === kind && selected.id === id

  return (
    <svg
      width={svgW}
      height={height}
      className="select-none text-[11px]"
      role="img"
      aria-label="Rota zaman çizelgesi"
      onClick={(e) => {
        if (e.target === e.currentTarget) onSelect(null)
      }}
    >
      <defs>
        {/* Wait stretches: same bar, dimmed and hatched, so the lane still
            reads as one run and the state colours keep their meaning. */}
        <pattern
          id="rota-wait-hatch"
          width={5}
          height={5}
          patternUnits="userSpaceOnUse"
          patternTransform="rotate(45)"
        >
          <rect width={5} height={5} fill="var(--color-surface)" opacity={0.55} />
          <line x1={0} y1={0} x2={0} y2={5} stroke="var(--color-surface)" strokeWidth={2} />
        </pattern>
      </defs>

      {/* Lane backgrounds + labels */}
      {rows.map((r) => (
        <g key={r.id}>
          <rect
            x={0}
            y={TOP_H + r.y * ROW_H}
            width={svgW}
            height={ROW_H}
            fill={r.y % 2 === 0 ? 'transparent' : 'var(--color-surface-2)'}
            opacity={0.5}
          />
          <RowLabel
            row={r}
            onClick={() => onSelect({ kind: 'session', id: r.id })}
            onOpenTrajectory={onOpenTrajectory}
          />
        </g>
      ))}

      {/* Collapsed dead air: a few-pixel sliver per skipped stretch. Too narrow
          for a hatch to read, so it is a dimmed band with a dashed seam. */}
      {scale.segments
        .filter((seg) => seg.gap)
        .map((seg) => (
          <g key={`gap:${seg.start}`}>
            <rect
              x={seg.x0}
              y={TOP_H - 6}
              width={seg.width}
              height={height - TOP_H + 6}
              fill="var(--color-surface-2)"
            />
            <line
              x1={seg.x0 + seg.width / 2}
              x2={seg.x0 + seg.width / 2}
              y1={TOP_H - 6}
              y2={height}
              stroke="var(--color-border)"
              strokeDasharray="2 3"
            />
            <title>{`${formatGapSpan(seg.end - seg.start)} boş · kırpıldı`}</title>
          </g>
        ))}

      {/* Future strip */}
      <rect
        x={xNow}
        y={0}
        width={FUTURE_W}
        height={height}
        fill="var(--color-accent-soft)"
        opacity={0.25}
      />
      <text x={xNow + 6} y={TOP_H - 9} fill="var(--color-text-dim)">
        gelecek
      </text>
      {future.map((f) => (
        <g key={f.id}>
          <line
            x1={x(f.at)}
            x2={x(f.at)}
            y1={TOP_H - 4}
            y2={height}
            stroke="var(--color-accent)"
            strokeDasharray="2 4"
            opacity={0.6}
          />
          <text x={x(f.at) + 3} y={TOP_H - 9} fill="var(--color-text)">
            <title>{`${f.label} · ${new Date(f.schedule.fireAt * 1000).toLocaleString()}`}</title>⏰{' '}
            {f.label.slice(0, 14)}
          </text>
        </g>
      ))}

      {/* Time ticks (past) */}
      <text x={LABEL_W + 2} y={TOP_H - 9} fill="var(--color-text-dim)">
        {fmtClock(t0)}
        {scale.collapsedSec > 0 && (
          <tspan fill="var(--color-text-dim)">
            {' '}
            · {formatGapSpan(scale.collapsedSec)} kırpıldı
          </tspan>
        )}
      </text>

      {/* Edges */}
      {edges.map((e) => {
        const y1 = rowY.get(e.from)
        const y2 = rowY.get(e.to)
        if (y1 === undefined || y2 === undefined) return null
        const ex = x(e.at)
        const back = e.kind === 'reported'
        const d = back
          ? `M ${ex} ${y1} C ${ex + 24} ${y1}, ${ex + 24} ${y2}, ${ex} ${y2}`
          : `M ${ex} ${y1} C ${ex - 16} ${y1}, ${ex - 16} ${y2}, ${ex} ${y2}`
        return (
          <path
            key={e.id}
            d={d}
            fill="none"
            stroke={EDGE_COLOR[e.kind]}
            strokeWidth={1.5}
            strokeDasharray={back ? '4 3' : undefined}
            opacity={0.9}
          >
            <title>{`${e.kind}: ${e.from} → ${e.to}`}</title>
          </path>
        )
      })}

      {/* Bars */}
      {bars.map((b) => {
        const cy = rowY.get(b.rowId)
        if (cy === undefined) return null
        const run = b.kind === 'flowrun'
        const h = run ? 5 : 11
        const y = run ? cy + 5 : cy - h / 2
        // Both ends come off the same axis, so a bar starts and ends exactly
        // where its timestamps say — the log axis bends the axis, not the bar.
        const x1 = x(b.start)
        const x2 = Math.max(x1 + MIN_BAR_PX, x(b.end))
        const sel = isSel(run ? 'flowrun' : 'session', run ? b.run!.runId : b.rowId)
        return (
          <g
            key={b.id}
            className="cursor-pointer"
            onClick={(ev) => {
              ev.stopPropagation()
              onSelect(
                run ? { kind: 'flowrun', id: b.run!.runId } : { kind: 'session', id: b.rowId },
              )
            }}
            onDoubleClick={(ev) => {
              ev.stopPropagation()
              if (run) onOpenFlowRun?.(b.run!.flowId)
              else onOpenSession?.(b.rowId)
            }}
          >
            <title>{`${b.label} · ${b.state} · ${fmtClock(b.start)} → ${b.live ? 'şimdi' : fmtClock(b.end)}`}</title>
            <rect
              x={x1}
              y={y}
              width={x2 - x1}
              height={h}
              rx={run ? 2 : 4}
              fill={barFill(b)}
              opacity={run ? 0.7 : 0.9}
              stroke={sel ? 'var(--color-text)' : 'none'}
              strokeWidth={sel ? 1.5 : 0}
            />
            {b.waits?.map((w) => {
              // Waits ride the same axis as the bar, so no extra mapping.
              const wx1 = Math.max(x1, x(w.start))
              const wx2 = Math.min(x2, x(w.end))
              // A wait that cleared the minimum duration deserves to be seen:
              // over a multi-day window a 20-minute wait is sub-pixel, so it
              // gets a floor and is nudged back inside the bar when the floor
              // would push it past the end.
              const ww = Math.max(WAIT_MIN_PX, wx2 - wx1)
              const wx = Math.min(wx1, Math.max(x1, x2 - ww))
              if (wx2 <= wx1 || x2 - x1 < WAIT_MIN_PX) return null
              return (
                <rect
                  key={`wait:${w.start}`}
                  x={wx}
                  y={y}
                  width={Math.min(ww, x2 - wx)}
                  height={h}
                  rx={2}
                  fill="url(#rota-wait-hatch)"
                >
                  <title>{formatWait(w)}</title>
                </rect>
              )
            })}
            {b.live && !run && (
              <circle cx={x2 - 3} cy={cy} r={3} fill="#fff" opacity={0.9}>
                <animate
                  attributeName="opacity"
                  values="0.9;0.2;0.9"
                  dur="1.6s"
                  repeatCount="indefinite"
                />
              </circle>
            )}
          </g>
        )
      })}

      {/* Marks */}
      {marks.map((m) => {
        const cy = rowY.get(m.rowId)
        if (cy === undefined) return null
        return (
          <text
            key={m.id}
            x={x(m.at) - 5}
            y={cy - 9}
            fill={
              m.kind === 'stall'
                ? '#ef4444'
                : m.kind === 'fired'
                  ? '#f59e0b'
                  : 'var(--color-text-dim)'
            }
          >
            <title>{m.label}</title>
            {markGlyph(m)}
          </text>
        )
      })}

      {/* Now line */}
      <line
        x1={xNow}
        x2={xNow}
        y1={TOP_H - 6}
        y2={height}
        stroke="var(--color-text)"
        strokeWidth={1}
        opacity={0.7}
      />
      <text x={xNow - 26} y={TOP_H - 9} fill="var(--color-text)">
        şimdi
      </text>
    </svg>
  )
}

function RowLabel({
  row,
  onClick,
  onOpenTrajectory,
}: {
  row: RotaRow
  onClick: () => void
  onOpenTrajectory?: (id: string) => void
}) {
  const s = row.session
  const y = TOP_H + row.y * ROW_H + ROW_H / 2 + 3.5
  const indent = row.depth === 0 ? 8 : 26
  const label = (s.title || s.id).slice(0, row.depth === 0 ? 22 : 19)
  return (
    <text x={indent} y={y} fill="var(--color-text)" className="cursor-pointer" onClick={onClick}>
      <title>{`${s.id} · ${s.kind}${s.origin ? ` · ${s.origin.kind}` : ''}${row.trajectory ? ` · rota ${row.trajectory.trajectoryId} rev ${row.trajectory.revision}` : ''}`}</title>
      <tspan fill="var(--color-text-dim)">{laneOriginGlyph(s)} </tspan>
      {label}
      {row.trajectory && (
        <tspan
          fill="var(--color-accent)"
          className="cursor-pointer"
          onClick={(e) => {
            e.stopPropagation()
            onOpenTrajectory?.(row.trajectory!.trajectoryId)
          }}
        >
          {' '}
          ◈
        </tspan>
      )}
    </text>
  )
}

function fmtClock(t: number): string {
  const d = new Date(t * 1000)
  return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`
}

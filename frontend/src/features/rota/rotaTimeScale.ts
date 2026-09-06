// Rota F0 time scale: the past window's seconds → pixels mapping, with the
// option to collapse "dead air" — stretches of the window where no lane has a
// bar, an edge or a mark. Long idle nights otherwise eat most of the canvas
// and squash the minutes that actually carry work into a few pixels. A bar that
// was split into sittings (rotaSegments) counts only those sittings as
// activity, so the idle hours INSIDE a bar collapse like any other dead air.
//
// It also carries the optional LOG duration axis: with `logDuration` on, time
// inside a live stretch is spent on a log scale, so a short session keeps a
// readable width while a very long one stops eating the panel. Unlike a
// bar-width trick this transforms the AXIS, so bars, edges, marks and wait
// slices all follow one mapping and every start/end stays exactly where its
// timestamp says.
//
// Pure numbers, no DOM: the canvas asks for a scale and calls it, the tests
// assert the mapping directly.
import type { RotaLayout } from './rotaLayout'

/** A stretch of the time window that carries no activity at all. */
export interface RotaGap {
  start: number
  end: number
}

/** One piece of the piecewise-linear mapping: [start,end] seconds occupying
 *  [x0,x0+width] pixels. `gap` pieces are the collapsed ones. */
interface ScaleSegment {
  start: number
  end: number
  x0: number
  width: number
  gap: boolean
}

export interface RotaTimeScale {
  /** Map a unix second inside the past window to a pixel offset. */
  x: (t: number) => number
  segments: ScaleSegment[]
  gaps: RotaGap[]
  /** Seconds hidden by collapsing. 0 when nothing was collapsed. */
  collapsedSec: number
}

/** Gaps shorter than this stay as they are: collapsing them would only add
 *  visual noise without buying back meaningful width. */
const MIN_GAP_SEC = 5 * 60

/** Pixels a collapsed gap keeps: just enough for the hatched sliver to read as
 *  a break, since every pixel spent here is one the real activity loses. */
export const GAP_PX = 4

/** Activity is padded by this much on each side before a gap is measured, so a
 *  bar's end and the next bar's start never touch the collapse marker. */
const EDGE_PAD_SEC = 30

/** Seconds below which the log axis is effectively linear. Short spans keep
 *  their true proportions; compression only takes hold well above this. Set to
 *  five minutes because most sessions already run for minutes — at a 60s knee
 *  the ordinary lanes were being compressed alongside the long ones, which is
 *  the opposite of the point. */
const LOG_KNEE_SEC = 5 * 60

/** Elapsed seconds → the axis's own units. Linear when `log` is off. `log1p`
 *  over the knee keeps the curve finite and monotonic at zero, so ordering and
 *  "later is further right" always hold. */
function measure(sec: number, log: boolean): number {
  const d = Math.max(0, sec)
  if (!log) return d
  return LOG_KNEE_SEC * Math.log1p(d / LOG_KNEE_SEC)
}

/** Every instant in the past window where something starts or ends. The log
 *  axis slices live stretches at these, so each INTERVAL between neighbouring
 *  events is measured on its own — that is what keeps a short session readable
 *  wherever it sits in the window, instead of being squashed just because the
 *  window's earlier seconds already spent the panel. */
function eventInstants(layout: RotaLayout): number[] {
  const { t0, now } = layout
  const set = new Set<number>()
  const add = (t: number) => {
    if (t >= t0 && t <= now) set.add(t)
  }
  for (const b of layout.bars) {
    add(b.start)
    add(b.end)
    // A split bar's sittings are cut points too, so each sitting is measured on
    // its own instead of sharing one interval with the idle time next to it.
    for (const s of b.segments ?? []) {
      add(s.start)
      add(s.end)
    }
  }
  for (const m of layout.marks) add(m.at)
  for (const e of layout.edges) add(e.at)
  return [...set].sort((a, b) => a - b)
}

/** Every instant in the past window that carries something, as [start,end]
 *  spans (a mark or an edge is a zero-length span). */
function activitySpans(layout: RotaLayout): RotaGap[] {
  const { t0, now } = layout
  const spans: RotaGap[] = []
  const push = (start: number, end: number) => {
    const s = Math.max(t0, Math.min(now, start))
    const e = Math.max(t0, Math.min(now, end))
    if (e >= s) spans.push({ start: s, end: e })
  }
  for (const b of layout.bars) {
    if (b.segments && b.segments.length > 0) {
      // A split bar spans createdAt → updatedAt but only WORKED during its
      // sittings, so the hours between them are dead air like any other and get
      // collapsed with them. Its own endpoints stay anchored as instants: they
      // are real timestamps (creation, last update) and the bar must not start
      // or end inside a collapse sliver.
      push(b.start, b.start)
      for (const s of b.segments) push(s.start, s.end)
      push(b.end, b.end)
      continue
    }
    push(b.start, b.end)
  }
  for (const m of layout.marks) push(m.at, m.at)
  for (const e of layout.edges) push(e.at, e.at)
  return spans
}

/** Empty stretches of [t0, now] longer than `minGapSec`, in time order. */
export function findGaps(layout: RotaLayout, minGapSec: number = MIN_GAP_SEC): RotaGap[] {
  const { t0, now } = layout
  if (now <= t0) return []
  const spans = activitySpans(layout)
  if (spans.length === 0) return []
  spans.sort((a, b) => a.start - b.start)

  const gaps: RotaGap[] = []
  // Sweep the merged spans; the hole between one merged span and the next is a
  // gap candidate. The window edges (t0 → first span, last span → now) count
  // too: a lane that has been idle all morning starts with dead air.
  let cursor = t0
  for (const s of spans) {
    const from = cursor + (cursor > t0 ? EDGE_PAD_SEC : 0)
    const to = s.start - EDGE_PAD_SEC
    if (to - from >= minGapSec) gaps.push({ start: from, end: to })
    if (s.end > cursor) cursor = s.end
  }
  const tailFrom = cursor + (cursor > t0 ? EDGE_PAD_SEC : 0)
  if (now - tailFrom >= minGapSec) gaps.push({ start: tailFrom, end: now })
  return gaps
}

/** Build the past-window scale. With `collapse` off (or nothing to collapse)
 *  this is the plain linear mapping the canvas has always used. */
export function buildTimeScale(
  layout: RotaLayout,
  opts: {
    x0: number
    width: number
    collapse: boolean
    minGapSec?: number
    /** Spend time on a log scale inside each live stretch, so long sessions
     *  stop eating the panel while short ones stay readable. */
    logDuration?: boolean
  },
): RotaTimeScale {
  const { x0, width, collapse } = opts
  const log = opts.logDuration ?? false
  const { t0, now } = layout
  const gaps = collapse ? findGaps(layout, opts.minGapSec ?? MIN_GAP_SEC) : []

  // Live stretches: the window minus the collapsed gaps. With no gaps that is
  // the whole window, which keeps the no-collapse path on the same code.
  const live: { start: number; end: number }[] = []
  let cursor = t0
  for (const g of gaps) {
    if (g.start > cursor) live.push({ start: cursor, end: g.start })
    cursor = g.end
  }
  if (now > cursor) live.push({ start: cursor, end: now })
  if (live.length === 0) live.push({ start: t0, end: Math.max(t0 + 1, now) })

  // Gaps take a fixed sliver each; the live stretches share what is left, in
  // proportion to their MEASURED length (log or linear). If the slivers alone
  // would fill the panel, fall back rather than squeezing activity to nothing.
  const collapsedSec = gaps.reduce((n, g) => n + (g.end - g.start), 0)
  const gapPx = gaps.length * GAP_PX
  if (gaps.length > 0 && gapPx >= width * 0.5) {
    return buildTimeScale(layout, { ...opts, collapse: false })
  }
  // On the log axis a live stretch is cut at every event instant, so each
  // interval between neighbouring events is measured on its own. Measuring a
  // whole stretch at once would spend the panel on its earliest seconds and
  // leave a short session near the window's end squashed regardless of how
  // long it actually ran.
  const cuts = log ? eventInstants(layout) : []
  const slices: { start: number; end: number }[] = []
  for (const l of live) {
    const inner = cuts.filter((c) => c > l.start && c < l.end)
    let from = l.start
    for (const c of inner) {
      slices.push({ start: from, end: c })
      from = c
    }
    slices.push({ start: from, end: l.end })
  }

  const totalUnits = slices.reduce((n, sl) => n + measure(sl.end - sl.start, log), 0) || 1
  const pxPerUnit = (width - gapPx) / totalUnits

  // Walk the window once, emitting slices and gaps in time order.
  const segments: ScaleSegment[] = []
  let px = x0
  let si = 0
  const emitSlice = (sl: { start: number; end: number }) => {
    const w = measure(sl.end - sl.start, log) * pxPerUnit
    segments.push({ start: sl.start, end: sl.end, x0: px, width: w, gap: false })
    px += w
  }
  for (const g of gaps) {
    while (si < slices.length && slices[si].start < g.start) emitSlice(slices[si++])
    segments.push({ start: g.start, end: g.end, x0: px, width: GAP_PX, gap: true })
    px += GAP_PX
  }
  while (si < slices.length) emitSlice(slices[si++])

  return {
    x: (t) => scaleAt(segments, clamp(t, t0, now), x0, px, log),
    segments,
    gaps,
    collapsedSec,
  }
}

function scaleAt(
  segments: ScaleSegment[],
  t: number,
  min: number,
  max: number,
  log: boolean,
): number {
  for (const s of segments) {
    if (t < s.start) return s.x0
    if (t <= s.end) {
      if (s.end <= s.start) return s.x0
      // Inside a live segment the offset follows the same measure as the
      // segment's width, which is what keeps every timestamp on the axis.
      const inner = s.gap
        ? ((t - s.start) / (s.end - s.start)) * s.width
        : (measure(t - s.start, log) / (measure(s.end - s.start, log) || 1)) * s.width
      return s.x0 + inner
    }
  }
  return segments.length === 0 ? min : max
}

function clamp(t: number, lo: number, hi: number): number {
  return t < lo ? lo : t > hi ? hi : t
}

/** "2 sa 15 dk" — how much time a collapse marker hides. */
export function formatGapSpan(sec: number): string {
  const m = Math.round(sec / 60)
  if (m < 60) return `${m} dk`
  const h = Math.floor(m / 60)
  const rest = m % 60
  if (h < 24) return rest ? `${h} sa ${rest} dk` : `${h} sa`
  const d = Math.floor(h / 24)
  const restH = h % 24
  return restH ? `${d} gün ${restH} sa` : `${d} gün`
}

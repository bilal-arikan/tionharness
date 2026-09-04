import { describe, expect, it } from 'vitest'
import type { RotaLayout } from './rotaLayout'
import { GAP_PX, buildTimeScale, findGaps, formatGapSpan } from './rotaTimeScale'

const NOW = 100_000

function layout(partial: Partial<RotaLayout>): RotaLayout {
  return {
    rows: [],
    bars: [],
    edges: [],
    marks: [],
    future: [],
    t0: NOW - 10_000,
    now: NOW,
    t1: NOW + 1800,
    ...partial,
  }
}

function bar(start: number, end: number) {
  return {
    id: 'bar:' + start,
    rowId: 'R',
    kind: 'session' as const,
    start,
    end,
    live: false,
    state: 'completed',
    label: 'R',
  }
}

describe('findGaps', () => {
  it('finds the dead air between two bars', () => {
    const l = layout({ bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 1_000, NOW)] })
    expect(findGaps(l)).toEqual([{ start: NOW - 8_970, end: NOW - 1_030 }])
  })

  it('ignores gaps shorter than the minimum', () => {
    const l = layout({ bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 8_800, NOW)] })
    expect(findGaps(l)).toEqual([])
  })

  it('counts leading and trailing dead air', () => {
    const l = layout({ bars: [bar(NOW - 5_100, NOW - 5_000)] })
    expect(findGaps(l)).toEqual([
      { start: NOW - 10_000, end: NOW - 5_130 },
      { start: NOW - 4_970, end: NOW },
    ])
  })

  it('marks and edges keep their instant alive', () => {
    const l = layout({
      bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 1_000, NOW)],
      marks: [{ id: 'm', rowId: 'R', at: NOW - 5_000, kind: 'fired', label: 'x' }],
    })
    expect(findGaps(l)).toEqual([
      { start: NOW - 8_970, end: NOW - 5_030 },
      { start: NOW - 4_970, end: NOW - 1_030 },
    ])
  })

  it('returns nothing when the store is empty', () => {
    expect(findGaps(layout({}))).toEqual([])
  })
})

describe('buildTimeScale', () => {
  const opts = { x0: 200, width: 800, collapse: true }

  it('is plain linear when collapsing is off', () => {
    const l = layout({ bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 1_000, NOW)] })
    const s = buildTimeScale(l, { ...opts, collapse: false })
    expect(s.gaps).toEqual([])
    expect(s.collapsedSec).toBe(0)
    expect(s.x(NOW - 10_000)).toBe(200)
    expect(s.x(NOW - 5_000)).toBe(600)
    expect(s.x(NOW)).toBe(1000)
  })

  it('gives a collapsed gap a fixed sliver and the rest to real activity', () => {
    const l = layout({ bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 1_000, NOW)] })
    const s = buildTimeScale(l, opts)
    expect(s.gaps).toHaveLength(1)
    expect(s.collapsedSec).toBe(7_940)
    // 2060 live seconds share the panel minus one sliver.
    const pxPerSec = (800 - GAP_PX) / 2_060
    expect(s.x(NOW - 10_000)).toBe(200)
    expect(s.x(NOW - 9_000)).toBeCloseTo(200 + 1_000 * pxPerSec, 6)
    // Across the gap the jump is exactly the sliver.
    expect(s.x(NOW - 1_030) - s.x(NOW - 8_970)).toBeCloseTo(GAP_PX, 6)
    expect(s.x(NOW)).toBeCloseTo(1000, 6)
  })

  it('stays monotonic and clamps outside the window', () => {
    const l = layout({
      bars: [bar(NOW - 10_000, NOW - 9_500), bar(NOW - 6_000, NOW - 5_800), bar(NOW - 200, NOW)],
    })
    const s = buildTimeScale(l, opts)
    let prev = -Infinity
    for (let t = NOW - 10_000; t <= NOW; t += 137) {
      const x = s.x(t)
      expect(x).toBeGreaterThanOrEqual(prev)
      prev = x
    }
    expect(s.x(NOW - 99_999)).toBe(200)
    expect(s.x(NOW + 99_999)).toBeCloseTo(1000, 6)
  })

  it('falls back to linear when the slivers would eat half the panel', () => {
    // Many short bars separated by collapsible gaps in a narrow panel: the
    // slivers alone would take 80 of the 120 available pixels.
    const bars = []
    for (let i = 0; i < 20; i++) {
      const start = NOW - 100_000 + i * 5_000
      bars.push(bar(start, start + 5))
    }
    const l = layout({ t0: NOW - 100_000, bars })
    expect(findGaps(l).length).toBeGreaterThan(9)
    const s = buildTimeScale(l, { x0: 0, width: 120, collapse: true })
    expect(s.gaps).toEqual([])
    expect(s.collapsedSec).toBe(0)
  })

  it('segments tile the panel end to end', () => {
    const l = layout({ bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 1_000, NOW)] })
    const s = buildTimeScale(l, opts)
    expect(s.segments[0].x0).toBe(200)
    const last = s.segments[s.segments.length - 1]
    expect(last.x0 + last.width).toBeCloseTo(1000, 6)
    for (let i = 1; i < s.segments.length; i++) {
      expect(s.segments[i].x0).toBeCloseTo(s.segments[i - 1].x0 + s.segments[i - 1].width, 6)
    }
  })
})

describe('formatGapSpan', () => {
  it('reads in minutes, hours and days', () => {
    expect(formatGapSpan(600)).toBe('10 dk')
    expect(formatGapSpan(7_200)).toBe('2 sa')
    expect(formatGapSpan(8_100)).toBe('2 sa 15 dk')
    expect(formatGapSpan(90_000)).toBe('1 gün 1 sa')
    expect(formatGapSpan(172_800)).toBe('2 gün')
  })
})

describe('buildTimeScale logDuration', () => {
  const opts = { x0: 0, width: 1000, collapse: false }

  // One lane spanning the whole window, so the scale has a single live stretch
  // and the assertions are about the mapping alone.
  const l = layout({ t0: NOW - 10_000, bars: [bar(NOW - 10_000, NOW)] })

  it('keeps both window edges pinned', () => {
    const s = buildTimeScale(l, { ...opts, logDuration: true })
    expect(s.x(NOW - 10_000)).toBe(0)
    expect(s.x(NOW)).toBeCloseTo(1000, 6)
  })

  it('gives early (short) time more room than linear, late time less', () => {
    const lin = buildTimeScale(l, { ...opts, logDuration: false })
    const log = buildTimeScale(l, { ...opts, logDuration: true })
    // The first 10 minutes of the window occupy far more of the panel on the
    // log axis; the last stretch is correspondingly compressed.
    const early = NOW - 10_000 + 600
    expect(log.x(early)).toBeGreaterThan(lin.x(early))
    const late = NOW - 600
    expect(log.x(late)).toBeGreaterThan(lin.x(late))
    // Ordering is untouched.
    expect(log.x(early)).toBeLessThan(log.x(late))
  })

  it('stays monotonic across the window', () => {
    const s = buildTimeScale(l, { ...opts, logDuration: true })
    let prev = -Infinity
    for (let t = NOW - 10_000; t <= NOW; t += 97) {
      const x = s.x(t)
      expect(x).toBeGreaterThanOrEqual(prev)
      prev = x
    }
  })

  it('compresses a long span far more than a short one', () => {
    // Two equal-length windows, one a minute wide and one a day wide: on the
    // log axis the day is nowhere near 1440x the minute's pixels-per-second.
    const minute = buildTimeScale(layout({ t0: NOW - 60, bars: [bar(NOW - 60, NOW)] }), {
      ...opts,
      logDuration: true,
    })
    const day = buildTimeScale(layout({ t0: NOW - 86_400, bars: [bar(NOW - 86_400, NOW)] }), {
      ...opts,
      logDuration: true,
    })
    // Both fill the same panel, so compare how much of it the first minute takes.
    expect(minute.x(NOW - 30)).toBeGreaterThan(400)
    expect(day.x(NOW - 86_400 + 60)).toBeLessThan(200)
  })

  it('works alongside gap collapsing: gaps keep their fixed sliver', () => {
    const withGap = layout({
      t0: NOW - 10_000,
      bars: [bar(NOW - 10_000, NOW - 9_000), bar(NOW - 1_000, NOW)],
    })
    const s = buildTimeScale(withGap, { x0: 0, width: 1000, collapse: true, logDuration: true })
    expect(s.gaps).toHaveLength(1)
    const gapSeg = s.segments.find((seg) => seg.gap)!
    expect(gapSeg.width).toBe(GAP_PX)
    // Segments still tile the panel end to end.
    const last = s.segments[s.segments.length - 1]
    expect(last.x0 + last.width).toBeCloseTo(1000, 6)
    expect(s.x(NOW - 10_000)).toBe(0)
    expect(s.x(NOW)).toBeCloseTo(1000, 6)
  })

  it('is the plain linear mapping when off', () => {
    const s = buildTimeScale(l, { ...opts, logDuration: false })
    expect(s.x(NOW - 5_000)).toBeCloseTo(500, 6)
  })
})

import { describe, it, expect } from 'vitest'
import { barSegments, segmentGaps, formatSegments } from './rotaSegments'

const HOUR = 3600

describe('barSegments', () => {
  it('keeps the bouts that split the bar', () => {
    const spans = [
      { start: 100, end: 400 },
      { start: 100 + 16 * HOUR, end: 100 + 17 * HOUR },
    ]
    expect(barSegments(spans, 100, 100 + 17 * HOUR, false)).toEqual(spans)
  })

  it('returns nothing when the activity collapses to one stretch', () => {
    expect(barSegments([{ start: 100, end: 400 }], 100, 400, false)).toEqual([])
  })

  it('returns nothing without activity data', () => {
    expect(barSegments(undefined, 100, 400, false)).toEqual([])
    expect(barSegments([], 100, 400, false)).toEqual([])
  })

  it('clips to the bar and merges what clipping made touch', () => {
    const spans = [
      { start: 0, end: 150 },
      { start: 160, end: 900 },
    ]
    // The bar starts at 200: both bouts collapse onto/behind it, so there is
    // nothing left to split.
    expect(barSegments(spans, 200, 900, false)).toEqual([])
  })

  it('carries the tail of a live bar to its end', () => {
    const spans = [
      { start: 100, end: 400 },
      { start: 5000, end: 5200 },
    ]
    const got = barSegments(spans, 100, 9000, true)
    expect(got[got.length - 1].end).toBe(9000)
    expect(got[0]).toEqual({ start: 100, end: 400 })
  })

  it('does not extend the tail of a finished bar', () => {
    const spans = [
      { start: 100, end: 400 },
      { start: 5000, end: 5200 },
    ]
    const got = barSegments(spans, 100, 9000, false)
    expect(got[got.length - 1].end).toBe(5200)
  })

  it('leaves the caller spans untouched', () => {
    const spans = [
      { start: 100, end: 400 },
      { start: 5000, end: 5200 },
    ]
    barSegments(spans, 200, 9000, true)
    expect(spans).toEqual([
      { start: 100, end: 400 },
      { start: 5000, end: 5200 },
    ])
  })
})

describe('segmentGaps', () => {
  it('reports the stretches between segments and the head gap', () => {
    const segments = [
      { start: 500, end: 800 },
      { start: 2000, end: 2400 },
    ]
    expect(segmentGaps(segments, 100)).toEqual([
      { start: 100, end: 500 },
      { start: 800, end: 2000 },
    ])
  })

  it('has no head gap when the first segment starts with the bar', () => {
    const segments = [
      { start: 100, end: 800 },
      { start: 2000, end: 2400 },
    ]
    expect(segmentGaps(segments, 100)).toEqual([{ start: 800, end: 2000 }])
  })
})

describe('formatSegments', () => {
  it('sums work and idle time', () => {
    const segments = [
      { start: 0, end: 30 * 60 },
      { start: 3 * HOUR, end: 3 * HOUR + 90 * 60 },
    ]
    expect(formatSegments(segments, 0)).toBe('2 oturuş · 2 sa 0 dk çalışma · 2 sa 30 dk boşluk')
  })

  it('is empty without segments', () => {
    expect(formatSegments([], 0)).toBe('')
  })
})

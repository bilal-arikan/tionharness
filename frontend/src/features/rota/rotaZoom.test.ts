import { describe, expect, it } from 'vitest'
import {
  MAX_ZOOM,
  MIN_ZOOM,
  ZOOM_STEPS,
  anchoredScrollLeft,
  clampZoom,
  formatZoom,
  stepZoom,
} from './rotaZoom'

describe('clampZoom', () => {
  it('holds the range and survives junk input', () => {
    expect(clampZoom(0.1)).toBe(MIN_ZOOM)
    expect(clampZoom(99)).toBe(MAX_ZOOM)
    expect(clampZoom(2)).toBe(2)
    expect(clampZoom(Number.NaN)).toBe(MIN_ZOOM)
    expect(clampZoom(Number.POSITIVE_INFINITY)).toBe(MIN_ZOOM)
  })
})

describe('stepZoom', () => {
  it('walks the steps up and down', () => {
    expect(stepZoom(1, 1)).toBe(1.5)
    expect(stepZoom(1.5, 1)).toBe(2)
    expect(stepZoom(2, -1)).toBe(1.5)
    expect(stepZoom(1.5, -1)).toBe(1)
  })

  it('stops at the ends instead of running off', () => {
    expect(stepZoom(MAX_ZOOM, 1)).toBe(MAX_ZOOM)
    expect(stepZoom(MIN_ZOOM, -1)).toBe(MIN_ZOOM)
  })

  it('lands on a step from an in-between (wheel) value', () => {
    // 2.4 came from a wheel gesture; stepping up goes to the next real step.
    expect(stepZoom(2.4, 1)).toBe(3)
    expect(stepZoom(2.4, -1)).toBe(2)
  })

  it('every step is reachable by walking up from the minimum', () => {
    const seen: number[] = [MIN_ZOOM]
    let z = MIN_ZOOM
    for (let i = 0; i < ZOOM_STEPS.length + 2; i++) {
      const next = stepZoom(z, 1)
      if (next === z) break
      seen.push(next)
      z = next
    }
    expect(seen).toEqual([...ZOOM_STEPS])
  })
})

describe('anchoredScrollLeft', () => {
  it('keeps the instant under the cursor fixed while zooming in', () => {
    // Cursor 300px into the viewport, container scrolled to 200: the content
    // under it is at 500 in canvas space. At 2x that content sits at 1000, so
    // scrollLeft must become 700 to leave it under the same cursor.
    expect(anchoredScrollLeft(200, 300, 1, 2)).toBe(700)
  })

  it('works the same way zooming out', () => {
    expect(anchoredScrollLeft(700, 300, 2, 1)).toBe(200)
  })

  it('never scrolls past the left edge', () => {
    expect(anchoredScrollLeft(0, 50, 2, 1)).toBe(0)
  })

  it('excludes the fixed label column from the scaling', () => {
    // Only the axis right of x=180 stretches. Cursor at canvas 500 sits 320
    // into the axis; at 2x that becomes 640, so the axis point lands at 820
    // and scrollLeft must be 820 - 300 = 520.
    expect(anchoredScrollLeft(200, 300, 1, 2, 180)).toBe(520)
    // Ignoring the fixed column would overshoot to 700.
    expect(anchoredScrollLeft(200, 300, 1, 2, 0)).toBe(700)
  })

  it('a cursor inside the label column does not scroll backwards', () => {
    expect(anchoredScrollLeft(0, 100, 1, 2, 180)).toBe(80)
  })
})

describe('formatZoom', () => {
  it('reads whole and fractional levels', () => {
    expect(formatZoom(1)).toBe('1x')
    expect(formatZoom(1.5)).toBe('1.5x')
    expect(formatZoom(8)).toBe('8x')
  })
})

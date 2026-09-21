import { describe, expect, it } from 'vitest'
import {
  DEFAULT_GAP_SEC,
  GAP_LADDER,
  ROTA_FUTURE_W,
  ROTA_LABEL_W,
  ROTA_PAD_R,
  activityGapSec,
  rotaPastWidth,
} from './rotaActivityGap'

const HOUR = 3600
const DAY = 24 * HOUR

describe('rotaPastWidth', () => {
  it('leaves the label column and the future strip out, then scales by zoom', () => {
    const fixed = ROTA_LABEL_W + ROTA_FUTURE_W + ROTA_PAD_R
    expect(rotaPastWidth(1000, 1)).toBe(1000 - fixed)
    expect(rotaPastWidth(1000, 2)).toBe((1000 - fixed) * 2)
  })

  it('floors the axis so a narrow panel still has somewhere to draw', () => {
    expect(rotaPastWidth(100, 1)).toBe(240)
    expect(rotaPastWidth(0, 1)).toBe(240)
  })
})

describe('activityGapSec', () => {
  it('always answers with a rung of the ladder', () => {
    for (const windowSec of [HOUR, 6 * HOUR, DAY, 30 * DAY]) {
      for (const px of [240, 800, 4000]) {
        expect(GAP_LADDER).toContain(activityGapSec(windowSec, px))
      }
    }
  })

  it('rounds the pixel requirement up to the next rung', () => {
    // 800px over 8h → 36 s/px → 12px wants 432s → 600 is the first rung above.
    expect(activityGapSec(8 * HOUR, 800)).toBe(600)
    // 800px over 4h → 18 s/px → 216s → 300, the lowest rung.
    expect(activityGapSec(4 * HOUR, 800)).toBe(300)
  })

  it('clamps at both ends of the ladder', () => {
    // A minute across a wide canvas asks for far less than the lowest rung.
    expect(activityGapSec(60, 4000)).toBe(GAP_LADDER[0])
    // A year across a narrow one asks for far more than the highest.
    expect(activityGapSec(365 * DAY, 240)).toBe(GAP_LADDER[GAP_LADDER.length - 1])
  })

  it('never raises the threshold as the axis is zoomed in', () => {
    const windowSec = 3 * DAY
    let previous = Number.POSITIVE_INFINITY
    for (const zoom of [1, 1.5, 2, 3, 4, 6, 8]) {
      const gap = activityGapSec(windowSec, rotaPastWidth(960, zoom))
      expect(gap).toBeLessThanOrEqual(previous)
      previous = gap
    }
    // The range is wide enough that zoom actually moves the answer.
    expect(previous).toBeLessThan(activityGapSec(windowSec, rotaPastWidth(960, 1)))
  })

  it('falls back to the server default when there is no window to measure', () => {
    expect(activityGapSec(0, 800)).toBe(DEFAULT_GAP_SEC)
    expect(activityGapSec(-HOUR, 800)).toBe(DEFAULT_GAP_SEC)
    expect(activityGapSec(HOUR, 0)).toBe(DEFAULT_GAP_SEC)
    expect(activityGapSec(Number.NaN, 800)).toBe(DEFAULT_GAP_SEC)
    expect(activityGapSec(HOUR, Number.POSITIVE_INFINITY)).toBe(DEFAULT_GAP_SEC)
  })
})

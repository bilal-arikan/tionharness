import { describe, expect, it } from 'vitest'
import { COL_W, LABEL_W, PAD, colWidth, columnsOverflow, svgWidth } from './trajectoryGeometry'

describe('trajectoryGeometry', () => {
  it('shares a wide panel evenly and never drops below the minimum', () => {
    expect(colWidth(4, 1200)).toBe(Math.floor((1200 - LABEL_W - PAD) / 4))
    expect(colWidth(8, 600)).toBe(COL_W)
    expect(colWidth(0, 600)).toBe(Math.max(COL_W, 600 - LABEL_W - PAD))
  })

  it('reports overflow only when the minimum widths exceed the panel', () => {
    // 4 phases + "fazsız" in the 1440px layout's ~912px panel: fits at 150/col.
    expect(columnsOverflow(5, 912)).toBe(false)
    // 8 columns cannot fit 912px at the minimum width → sideways scroll.
    expect(columnsOverflow(8, 912)).toBe(true)
    expect(svgWidth(8, 912)).toBe(LABEL_W + 8 * COL_W + PAD)
  })
})

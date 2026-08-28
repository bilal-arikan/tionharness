import { describe, expect, it } from 'vitest'
import {
  commitCountLabel,
  commitLevel,
  heatmapMove,
  heatmapTooltipReducer,
  initialHeatmapTooltipState,
  isHeatmapTooltipVisible,
} from './commitHeatmapModel'

describe('CommitHeatmap', () => {
  it('maps zero and non-zero counts to five stable intensity levels', () => {
    expect([0, 1, 3, 5, 10].map((count) => commitLevel(count, 10))).toEqual([0, 1, 2, 2, 4])
  })

  it('moves between day rows and week columns without leaving the grid', () => {
    expect(heatmapMove(8, 'ArrowUp', 20)).toBe(7)
    expect(heatmapMove(8, 'ArrowDown', 20)).toBe(9)
    expect(heatmapMove(8, 'ArrowLeft', 20)).toBe(1)
    expect(heatmapMove(8, 'ArrowRight', 20)).toBe(15)
    expect(heatmapMove(0, 'ArrowLeft', 20)).toBe(0)
    expect(heatmapMove(19, 'ArrowRight', 20)).toBe(19)
  })

  it('moves to the first and last day with Home and End', () => {
    expect(heatmapMove(8, 'Home', 20)).toBe(0)
    expect(heatmapMove(8, 'End', 20)).toBe(19)
  })

  it('provides the exact count without relying on color', () => {
    expect(commitCountLabel(3)).toBe('3 commit')
  })

  it('opens tooltips from focus and hover and closes each interaction independently', () => {
    let state = heatmapTooltipReducer(initialHeatmapTooltipState, { type: 'focus', index: 4 })
    expect(isHeatmapTooltipVisible(state, 4)).toBe(true)

    state = heatmapTooltipReducer(state, { type: 'mouseenter', index: 4 })
    state = heatmapTooltipReducer(state, { type: 'blur', index: 4 })
    expect(isHeatmapTooltipVisible(state, 4)).toBe(true)

    state = heatmapTooltipReducer(state, { type: 'mouseleave', index: 4 })
    expect(isHeatmapTooltipVisible(state, 4)).toBe(false)
  })

  it('dismisses a tooltip with Escape until a new focus or hover interaction', () => {
    let state = heatmapTooltipReducer(initialHeatmapTooltipState, { type: 'focus', index: 6 })
    state = heatmapTooltipReducer(state, { type: 'escape', index: 6 })
    expect(isHeatmapTooltipVisible(state, 6)).toBe(false)

    state = heatmapTooltipReducer(state, { type: 'mouseenter', index: 6 })
    expect(isHeatmapTooltipVisible(state, 6)).toBe(true)
  })
})

export function commitLevel(count: number, peak: number): number {
  if (count <= 0 || peak <= 0) return 0
  return Math.min(4, Math.max(1, Math.ceil((count / peak) * 4)))
}

export function heatmapMove(index: number, key: string, length: number): number {
  if (key === 'Home') return 0
  if (key === 'End') return length - 1
  const delta = key === 'ArrowLeft' ? -7 : key === 'ArrowRight' ? 7 : key === 'ArrowUp' ? -1 : 1
  return Math.min(length - 1, Math.max(0, index + delta))
}

export function commitCountLabel(count: number): string {
  return `${count} commit`
}

export interface HeatmapTooltipState {
  focused: number | null
  hovered: number | null
  dismissed: number | null
}

export type HeatmapTooltipAction =
  | { type: 'focus' | 'mouseenter'; index: number }
  | { type: 'blur' | 'mouseleave' | 'escape'; index: number }

export const initialHeatmapTooltipState: HeatmapTooltipState = {
  focused: null,
  hovered: null,
  dismissed: null,
}

export function heatmapTooltipReducer(
  state: HeatmapTooltipState,
  action: HeatmapTooltipAction,
): HeatmapTooltipState {
  switch (action.type) {
    case 'focus':
      return { ...state, focused: action.index, dismissed: null }
    case 'mouseenter':
      return { ...state, hovered: action.index, dismissed: null }
    case 'blur':
      return { ...state, focused: state.focused === action.index ? null : state.focused }
    case 'mouseleave':
      return { ...state, hovered: state.hovered === action.index ? null : state.hovered }
    case 'escape':
      return { ...state, dismissed: action.index }
  }
}

export function isHeatmapTooltipVisible(state: HeatmapTooltipState, index: number): boolean {
  return state.dismissed !== index && (state.focused === index || state.hovered === index)
}

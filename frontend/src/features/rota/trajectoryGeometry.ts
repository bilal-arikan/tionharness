// Pixel geometry of the per-trajectory view (Rota F1b): phase columns × lanes.
// Pure numbers so the SVG and its header agree on one layout, and so "does the
// canvas overflow the panel" is testable without a DOM.

/** Minimum column width. Was 210, which pushed the 5th column ("fazsız": nodes
 *  with no phase — end-of-run watchers and their sessions) off-screen at 1440px
 *  with nothing hinting it was there (observed 2026-09-03). */
export const COL_W = 150
export const ROW_H = 46
export const LABEL_W = 150
export const HEAD_H = 54
export const NODE_H = 26
export const PAD = 12

/** Column width: share the panel evenly, never below COL_W. */
export function colWidth(columns: number, width: number): number {
  return Math.max(COL_W, Math.floor((width - LABEL_W - PAD) / Math.max(1, columns)))
}

/** Total SVG width for `columns` columns in a panel `width` px wide. */
export function svgWidth(columns: number, width: number): number {
  return LABEL_W + columns * colWidth(columns, width) + PAD
}

/** True when the columns do not fit the panel and the view scrolls sideways. */
export function columnsOverflow(columns: number, width: number): boolean {
  return svgWidth(columns, width) > width
}

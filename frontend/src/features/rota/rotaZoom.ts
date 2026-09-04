// Rota F0 zoom: how many times wider than the panel the time axis is drawn.
// At 1x the past window fits exactly; above that the canvas overflows and the
// scroll container pans it, which is what lets a busy afternoon be inspected
// without changing the window filter.
//
// Zoom multiplies the axis only — row pitch, the label column and the future
// strip keep their pixel sizes, so text stays readable at every level and the
// lane labels never drift away from their bars.

/** Zoom levels, coarse enough that a click always makes a visible difference. */
export const ZOOM_STEPS = [1, 1.5, 2, 3, 4, 6, 8] as const

export const MIN_ZOOM = ZOOM_STEPS[0]
export const MAX_ZOOM = ZOOM_STEPS[ZOOM_STEPS.length - 1]

/** Clamp an arbitrary factor into range (wheel zoom is continuous). */
export function clampZoom(z: number): number {
  if (!Number.isFinite(z)) return MIN_ZOOM
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, z))
}

/** The next step up/down from `z`, for the +/- buttons and keyboard. */
export function stepZoom(z: number, dir: 1 | -1): number {
  const eps = 1e-6
  if (dir > 0) return ZOOM_STEPS.find((s) => s > z + eps) ?? MAX_ZOOM
  const below = ZOOM_STEPS.filter((s) => s < z - eps)
  return below.length > 0 ? below[below.length - 1] : MIN_ZOOM
}

/** "2x" / "1.5x" for the toolbar readout. */
export function formatZoom(z: number): string {
  return `${Number.isInteger(z) ? z : z.toFixed(1)}x`
}

/** Keep the time under the cursor fixed while the axis grows around it.
 *
 *  `anchor` is the cursor's offset from the scroll container's left edge and
 *  `scrollLeft` where the container currently sits. `fixedLeft` is the part of
 *  the canvas that does NOT scale with zoom (the label column): only the axis
 *  to its right stretches, so the ratio must be applied to the offset within
 *  that axis, not to the raw canvas offset. Getting this wrong leaves a small
 *  drift that grows with the label width.
 *
 *  Returns the scrollLeft that puts the same instant back under the cursor. */
export function anchoredScrollLeft(
  scrollLeft: number,
  anchor: number,
  from: number,
  to: number,
  fixedLeft = 0,
): number {
  const ratio = to / from
  // Canvas-space position of the cursor, then the same point expressed as an
  // offset into the scaling region.
  const at = scrollLeft + anchor
  const inAxis = Math.max(0, at - fixedLeft)
  return Math.max(0, fixedLeft + inAxis * ratio - anchor)
}

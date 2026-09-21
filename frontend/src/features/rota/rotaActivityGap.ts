// How long an idle stretch has to be before the Rota canvas treats it as the
// end of a sitting (_Docs/78 §16). The server's own default is 600s, but a
// fixed threshold is wrong at both ends of the zoom range: over a week-wide
// window every 10-minute pause is a sub-pixel sliver that only fragments the
// bar, while at 8x over an afternoon the same threshold hides the coffee
// breaks the zoom was opened to see.
//
// So the threshold follows the axis: a gap is worth splitting a bar for only
// if it would be wide enough to actually see. The result is bucketed onto a
// short ladder rather than tracked continuously — every distinct value is one
// more round trip to /api/sessions/activity (the endpoint has no cache), and
// the ladder keeps a slow wheel-zoom from firing a request per frame.

/** Lane label column plus the future strip and right padding: the part of the
 *  canvas that does NOT carry past time. Kept here so the panel can derive the
 *  same past width the canvas draws with, from `width` and `zoom` alone. */
export const ROTA_LABEL_W = 180
export const ROTA_FUTURE_W = 140
export const ROTA_PAD_R = 8
/** Floor for the past axis, so a very long history still scrolls instead of
 *  collapsing to nothing on a narrow panel. */
const MIN_PAST_W = 240

/** Pixels of width an idle stretch needs before splitting a bar along it says
 *  anything; below this the split is noise. */
const MIN_GAP_PX = 12

/** The thresholds we are willing to ask the server for, in seconds. Coarse on
 *  purpose: 6 rungs across the whole zoom range means at most 6 distinct
 *  requests for a session, however much the user scrubs the zoom. */
export const GAP_LADDER = [300, 600, 900, 1800, 3600, 7200] as const

/** Used when the window is not measurable yet (empty layout, zero width). */
export const DEFAULT_GAP_SEC = 600

/** Width of the past (time-carrying) part of the canvas, at this zoom.
 *  Mirrors RotaCanvas's own `pastW`; both read it from here so the panel's
 *  seconds-per-pixel is the one actually drawn. */
export function rotaPastWidth(width: number, zoom: number): number {
  return Math.max(MIN_PAST_W, width - ROTA_LABEL_W - ROTA_FUTURE_W - ROTA_PAD_R) * zoom
}

/** The sitting threshold for a window of `windowSec` drawn across
 *  `pastWidthPx` pixels: the smallest ladder rung at least MIN_GAP_PX wide,
 *  clamped to the ladder's ends.
 *
 *  Zooming in shrinks seconds-per-pixel, so the raw requirement falls and the
 *  chosen rung can only stay equal or go down — shorter pauses become visible,
 *  never the reverse.
 *
 *  Inputs that carry no window (a layout with no lanes, a canvas not measured
 *  yet) fall back to the server default; genuinely broken numbers are not
 *  distinguishable from those here, so the fallback covers both rather than
 *  pretending to a precision the caller does not have. */
export function activityGapSec(windowSec: number, pastWidthPx: number): number {
  if (!Number.isFinite(windowSec) || !Number.isFinite(pastWidthPx)) return DEFAULT_GAP_SEC
  if (windowSec <= 0 || pastWidthPx <= 0) return DEFAULT_GAP_SEC
  const secPerPx = windowSec / pastWidthPx
  const wanted = secPerPx * MIN_GAP_PX
  return GAP_LADDER.find((rung) => rung >= wanted) ?? GAP_LADDER[GAP_LADDER.length - 1]
}

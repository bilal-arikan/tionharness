// Viewport classification shared by the layout hooks and the CSS layer.
//
// Four WIDTH tiers drive the shell layout (which columns dock, which become
// drawers, how wide the reading measure is):
//   narrow  < 768px   portrait phone: bottom nav + slide-in drawers
//   square  768-1279  tablets / half-screen windows: icon rail, capped list
//                     columns, the detail panel becomes a drawer
//   wide    1280-1919 laptop / desktop: every column docks
//   ultra   >= 1920   large / ultra-wide monitors: docked columns plus a
//                     centred reading measure so lines stay readable
// The tier boundaries mirror Tailwind's `md` / `xl` breakpoints and the custom
// `3xl` breakpoint declared in index.css, so `square:` / `3xl:` utilities and
// the JS tiers always agree.
//
// The ASPECT axis is independent of width: a 1200x1600 portrait monitor is
// "square"-tier by width but has far more vertical than horizontal room, so a
// docked right-hand panel is a bad trade there even at wide widths.

export type ViewportTier = 'narrow' | 'square' | 'wide' | 'ultra'
export type ViewportAspect = 'portrait' | 'square' | 'landscape'

export const TIER_MIN_WIDTH: Record<Exclude<ViewportTier, 'narrow'>, number> = {
  square: 768,
  wide: 1280,
  ultra: 1920,
}

// Width / height ratio thresholds for the aspect axis. 1280x1024 (1.25) still
// counts as square; 16:10 and wider is landscape; taller than 9:10 is portrait.
const ASPECT_PORTRAIT_MAX = 0.9
const ASPECT_SQUARE_MAX = 1.25

export interface ViewportClass {
  tier: ViewportTier
  aspect: ViewportAspect
}

export function classifyWidth(width: number): ViewportTier {
  if (width >= TIER_MIN_WIDTH.ultra) return 'ultra'
  if (width >= TIER_MIN_WIDTH.wide) return 'wide'
  if (width >= TIER_MIN_WIDTH.square) return 'square'
  return 'narrow'
}

export function classifyAspect(width: number, height: number): ViewportAspect {
  if (height <= 0) return 'landscape'
  const ratio = width / height
  if (ratio < ASPECT_PORTRAIT_MAX) return 'portrait'
  if (ratio <= ASPECT_SQUARE_MAX) return 'square'
  return 'landscape'
}

export function classifyViewport(width: number, height: number): ViewportClass {
  return { tier: classifyWidth(width), aspect: classifyAspect(width, height) }
}

// Widest a drag-resizable list column may be per tier. On the square tier the
// rail + list + content must share <= 1279px, so a 640px list column (the drag
// maximum) would leave the content pane unusable; the cap is applied on top of
// the persisted width, which is kept intact for wider tiers.
const LIST_COLUMN_MAX_BY_TIER: Record<ViewportTier, number> = {
  narrow: Number.POSITIVE_INFINITY, // mobile drawers size themselves via CSS
  square: 256,
  wide: Number.POSITIVE_INFINITY,
  ultra: Number.POSITIVE_INFINITY,
}

export function capListColumnWidth(width: number, tier: ViewportTier): number {
  return Math.min(width, LIST_COLUMN_MAX_BY_TIER[tier])
}

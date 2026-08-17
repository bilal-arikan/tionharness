// Shared composer control styles — ONE standard for every send-row control so the
// input toolbar stays visually uniform (same radius, height and padding). Text
// actions share BTN_BASE; only the colour/semantic differs. Icon-only controls
// (attach, pickers) use BTN_ICON.
//
// BTN_BASE is responsive: below the `sm` breakpoint the label is hidden (see
// ActionButton) and the horizontal padding collapses to the icon-only width, so
// the send cluster keeps its square icon shape on narrow viewports instead of
// wrapping onto a second toolbar row.
export const BTN_BASE =
  'flex items-center justify-center gap-1.5 rounded-xl px-2.5 py-3 text-sm font-medium transition sm:px-4'
export const BTN_PRIMARY = `${BTN_BASE} bg-[var(--color-accent)] text-white hover:opacity-90 disabled:opacity-30`
export const BTN_DANGER = `${BTN_BASE} bg-[var(--color-danger)] text-white hover:opacity-90`
export const BTN_WARNING = `${BTN_BASE} bg-[var(--color-warning)] text-white hover:opacity-90`
export const BTN_SECONDARY = `${BTN_BASE} border border-[var(--color-border)] text-[var(--color-text)] hover:border-[var(--color-accent)]`
// Queue action ("Sıraya") — a bluish dark fill so it reads as a distinct, calm
// "later" action next to the accent Send / warning Interrupt buttons.
export const BTN_QUEUE = `${BTN_BASE} bg-[#1e3a5f] text-white hover:bg-[#264a75]`
// Compact twin of BTN_DANGER for the status strips that sit where the composer
// would be (worker running, pending self-wake): same danger fill, radius and
// hover as the composer's "Durdur", just a strip-sized height.
export const BTN_STOP_COMPACT =
  'inline-flex shrink-0 items-center gap-1.5 rounded-xl bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-white transition hover:opacity-90'
export const BTN_ICON =
  'rounded-xl border border-[var(--color-border)] px-2.5 py-3 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-30'

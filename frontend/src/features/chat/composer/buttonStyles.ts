// Shared composer control styles — ONE standard for every send-row control so the
// input toolbar stays visually uniform (same radius, height and padding). Text
// actions share BTN_BASE; only the colour/semantic differs. Icon-only controls
// (attach, pickers) use BTN_ICON.
export const BTN_BASE = 'rounded-xl px-4 py-3 text-sm font-medium transition'
export const BTN_PRIMARY = `${BTN_BASE} bg-[var(--color-accent)] text-white hover:opacity-90 disabled:opacity-30`
export const BTN_DANGER = `${BTN_BASE} bg-[var(--color-danger)] text-white hover:opacity-90`
export const BTN_WARNING = `${BTN_BASE} bg-[var(--color-warning)] text-white hover:opacity-90`
export const BTN_SECONDARY = `${BTN_BASE} border border-[var(--color-border)] text-[var(--color-text)] hover:border-[var(--color-accent)]`
export const BTN_ICON =
  'rounded-xl border border-[var(--color-border)] px-2.5 py-3 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-30'

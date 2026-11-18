// Shared composer control styles — ONE standard for every send-row control so the
// input toolbar stays visually uniform (same radius, height and padding). Text
// actions share BTN_BASE; only the colour/semantic differs. Icon-only controls
// (attach, pickers) use BTN_ICON.
//
// BTN_BASE is responsive: below the `sm` breakpoint the label is hidden (see
// ActionButton) and the horizontal padding collapses to the icon-only width, so
// the send cluster keeps its square icon shape on narrow viewports instead of
// wrapping onto a second toolbar row.
//
// CONTROL_H is the single height every toolbar control shares (agent picker,
// per-turn pickers, workdir badge, icon buttons, send cluster). Controls set it
// explicitly instead of deriving their height from padding, so a two-line
// trigger and an icon button can never drift apart. Touch-sized below `md`.
export const CONTROL_H = 'h-11 md:h-10'
const BTN_BASE = `flex ${CONTROL_H} items-center justify-center gap-1.5 rounded-xl px-2.5 text-sm font-medium transition sm:px-4`
export const BTN_PRIMARY = `${BTN_BASE} bg-[var(--color-accent)] text-[var(--color-on-accent)] hover:opacity-90 disabled:opacity-30`
export const BTN_DANGER = `${BTN_BASE} bg-[var(--color-danger)] text-[var(--color-on-danger)] hover:opacity-90`
export const BTN_WARNING = `${BTN_BASE} bg-[var(--color-warning)] text-[var(--color-on-warning)] hover:opacity-90`
// Queue action ("Sıraya") — the muted informational fill so it reads as a
// distinct, calm "later" action next to the accent Send / warning Interrupt
// buttons. Themed per mode: dark navy on dark, pale blue on light.
export const BTN_QUEUE = `${BTN_BASE} bg-[var(--color-info-soft)] text-[var(--color-text)] hover:opacity-90`
// Compact twin of BTN_DANGER for the status strips that sit where the composer
// would be (worker running, pending self-wake): same danger fill, radius and
// hover as the composer's "Durdur", just a strip-sized height.
export const BTN_STOP_COMPACT =
  'inline-flex shrink-0 items-center gap-1.5 rounded-xl bg-[var(--color-danger)] px-3 py-1.5 text-xs font-medium text-[var(--color-on-danger)] transition hover:opacity-90'
// Icon-button geometry without any colour. Each state below is a COMPLETE colour
// set on top of it: two border-* / text-* utilities on one element are resolved
// by stylesheet order, not class-string order, so "BTN_ICON + an override class"
// silently loses.
const BTN_ICON_BASE = `flex ${CONTROL_H} w-11 shrink-0 items-center justify-center rounded-xl border transition disabled:opacity-30 md:w-10`
// Resting icon button.
export const BTN_ICON = `${BTN_ICON_BASE} border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]`
// The control is open / toggled on.
export const BTN_ICON_ON = `${BTN_ICON_BASE} border-[var(--color-accent)] text-[var(--color-accent)]`
// Busy without being a warning (e.g. transcribing).
export const BTN_ICON_BUSY = `${BTN_ICON_BASE} border-[var(--color-border)] text-[var(--color-accent)]`
// Recording / destructive-while-active.
export const BTN_ICON_DANGER = `${BTN_ICON_BASE} animate-pulse border-[var(--color-danger)] text-[var(--color-danger)]`
// Shared frame for the labelled triggers (agent picker, per-turn pickers,
// workdir badge). The accent is reserved for the send action and for a control
// that is currently open, so a trigger signals a non-default value with full-ink
// text instead of an accent border.
export const TRIGGER_FRAME = `flex ${CONTROL_H} min-w-11 items-center justify-center rounded-xl border border-[var(--color-border)] px-2.5 text-sm transition hover:border-[var(--color-text-dim)] md:min-w-10`
export const TRIGGER_DEFAULT = 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
export const TRIGGER_SET = 'text-[var(--color-text)]'

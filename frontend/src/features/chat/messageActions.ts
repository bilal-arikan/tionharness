// Shared styling for the per-message footer: the passive meta (time, duration,
// model, token spend) and the action controls (rewind / delete / rate /
// read-aloud / retry).
//
// The footer sits OUTSIDE the bubble, directly beneath it, on the neutral chat
// background — so there is a single palette and no per-surface variants.
//
// The actions used to be hover-only ghosts (`opacity-0 group-hover:opacity-100`),
// which made them easy to miss. They now stay visible at rest, so each needs a
// real chip look that reads as a button.
//
// Why a module instead of inline classes: the same chip is used by five call sites,
// and Tailwind conflict resolution is by stylesheet order (NOT class-string order) —
// so "append an override class" is unreliable. Every state therefore returns a
// COMPLETE, non-conflicting class set.

// intent colours the hover/active treatment by what the action means.
export type ActionIntent = 'default' | 'danger' | 'positive'

const BASE =
  'inline-flex shrink-0 items-center gap-1 rounded-md border px-1.5 py-1 text-[11px] font-medium leading-none transition'

// Resting look: visible but quiet — a real affordance that does not compete with
// the message text.
const REST = 'border-[var(--color-border)] bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'

const HOVER: Record<ActionIntent, string> = {
  default: 'hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]',
  danger: 'hover:border-[var(--color-danger)] hover:text-[var(--color-danger)]',
  positive: 'hover:border-[var(--color-success)] hover:text-[var(--color-success)]',
}

// Turned-ON look (thumb chosen, read-aloud playing): a filled chip so the state is
// obvious without hovering. Replaces REST wholesale rather than layering on top.
const ACTIVE: Record<ActionIntent, string> = {
  default: 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]',
  danger:
    'border-[var(--color-danger)] bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)] text-[var(--color-danger)]',
  positive:
    'border-[var(--color-success)] bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
}

// actionChip is the resting (idle) style for one action button.
export function actionChip(intent: ActionIntent = 'default', extra = ''): string {
  return [BASE, REST, HOVER[intent], extra].filter(Boolean).join(' ')
}

// actionChipActive is the same chip in its toggled-on state.
export function actionChipActive(intent: ActionIntent = 'default', extra = ''): string {
  return [BASE, ACTIVE[intent], extra].filter(Boolean).join(' ')
}

const FOOTER_BASE = 'flex flex-wrap items-center gap-x-3 gap-y-1.5 px-1'

// TURN_FOOTER is the row BELOW a full-width bubble (assistant): passive meta pinned
// LEFT, action chips pinned RIGHT. Both clusters wrap independently, so a narrow
// column stacks them instead of overflowing. With only one cluster present,
// justify-between still parks it on its own side.
export const TURN_FOOTER = `${FOOTER_BASE} justify-between`

// TURN_FOOTER_END is the same row under a RIGHT-HUGGING bubble (the user's, capped
// at 80% width): the whole cluster is right-aligned so it stays visually attached
// to the bubble instead of stranding the meta on the far left of the column.
// A separate constant, NOT `TURN_FOOTER + 'justify-end'`: two justify-* classes on
// one element are resolved by stylesheet order, not class-string order.
export const TURN_FOOTER_END = `${FOOTER_BASE} justify-end`

// META_CLUSTER groups the read-only meta bits on the footer's left.
export const META_CLUSTER = 'flex min-w-0 flex-wrap items-center gap-2'

// ACTION_CLUSTER groups the action chips on the footer's right.
export const ACTION_CLUSTER = 'flex flex-wrap items-center gap-1'

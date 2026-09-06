// The shared shell for every panel that docks above the composer (todo list,
// pending tray, wait banners, ask/permission/plan prompts). All of them float over
// the transcript on their own shadow — no opaque strip behind them — and the
// negative bottom margin rests the bubble against the composer below.
//
// Only the TONE differs per panel, so the geometry lives here once and each caller
// picks a tint. Tones are color-mixed onto --color-surface so they hold up in both
// the light and dark themes.
import type { ReactNode } from 'react'

type ComposerCardTone =
  // Neutral surface — the todo checklist.
  | 'plain'
  // Faint grey — staged/queued interventions.
  | 'muted'
  // Accent — running workers.
  | 'worker'
  // Faint accent — the agent's question.
  | 'ask'
  // Amber — a timed auto-resume is armed.
  | 'wake'
  // Stronger amber — an approval gate, deliberately more urgent than 'wake'.
  | 'permission'
  // Green — a plan awaiting approval.
  | 'plan'

// Full class strings (not built by interpolation) so Tailwind's scanner sees them.
const TONE: Record<ComposerCardTone, string> = {
  plain: 'border-[var(--color-border)] bg-[var(--color-surface)]',
  muted:
    'border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-text-dim)_10%,var(--color-surface))]',
  worker:
    'border-[color-mix(in_srgb,var(--color-accent)_35%,var(--color-border))] bg-[var(--color-accent-soft)]',
  ask: 'border-[var(--color-accent)] bg-[color-mix(in_srgb,var(--color-accent)_8%,var(--color-surface))]',
  wake: 'border-[color-mix(in_srgb,var(--color-warning)_35%,var(--color-border))] bg-[color-mix(in_srgb,var(--color-warning)_12%,var(--color-surface))]',
  permission:
    'border-[var(--color-warning)]/70 bg-[color-mix(in_srgb,var(--color-warning)_18%,var(--color-surface))]',
  plan: 'border-[var(--color-success)] bg-[color-mix(in_srgb,var(--color-success)_10%,var(--color-surface))]',
}

interface Props {
  tone: ComposerCardTone
  // Extra classes for the card itself (padding, layout, overflow) — the caller
  // owns its inner spacing since the panels differ (the todo list draws its own
  // rows edge-to-edge, the banners want px-3 py-2).
  className?: string
  children: ReactNode
}

export function ComposerCard({ tone, className = '', children }: Props) {
  return (
    <div className="th-measure th-measure-card -mb-2 pt-2">
      <div
        className={`rounded-2xl rounded-b-lg border shadow-[var(--shadow-lg)] ${TONE[tone]} ${className}`}
      >
        {children}
      </div>
    </div>
  )
}

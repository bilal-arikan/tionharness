import type { ReactNode } from 'react'

// Badge — a small status pill. Tones map to theme tokens via color-mix so the
// tint re-themes with the palette (mirrors the StatusPill pattern that was
// hand-rolled in several panels).
export type BadgeTone = 'accent' | 'success' | 'warning' | 'danger' | 'muted'

const TONES: Record<BadgeTone, string> = {
  accent: 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]',
  success:
    'bg-[color-mix(in_srgb,var(--color-success)_16%,transparent)] text-[var(--color-success)]',
  warning:
    'bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] text-[var(--color-warning)]',
  danger:
    'bg-[color-mix(in_srgb,var(--color-danger)_16%,transparent)] text-[var(--color-danger)]',
  muted: 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]',
}

interface Props {
  tone?: BadgeTone
  className?: string
  children: ReactNode
}

export function Badge({ tone = 'muted', className = '', children }: Props) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium ${TONES[tone]} ${className}`}
    >
      {children}
    </span>
  )
}

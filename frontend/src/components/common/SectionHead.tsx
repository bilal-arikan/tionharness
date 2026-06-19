import type { ReactNode } from 'react'

// SectionHead — the uppercase, dim, letter-spaced label used to title a list or
// a group of fields. Replaces the `text-xs font-semibold uppercase tracking-wide
// text-[var(--color-text-dim)]` string duplicated across 20+ panels.
interface Props {
  className?: string
  children: ReactNode
}

export function SectionHead({ className = '', children }: Props) {
  return (
    <h3
      className={`text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)] ${className}`}
    >
      {children}
    </h3>
  )
}

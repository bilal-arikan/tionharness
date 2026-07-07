import type { ButtonHTMLAttributes } from 'react'

// IconButton — a square, bordered icon-only control used in panel/modal headers
// and list rows. Defaults to the dim → text hover treatment shared across the UI.
interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  // bare drops the border/padding for inline icon affordances (e.g. row actions).
  bare?: boolean
}

export function IconButton({ bare = false, className = '', ...props }: Props) {
  const base = bare
    ? 'rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-text)] disabled:opacity-40'
    : 'rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-40'
  return <button className={`${base} ${className}`} {...props} />
}

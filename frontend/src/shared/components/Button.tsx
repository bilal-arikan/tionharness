import type { ButtonHTMLAttributes } from 'react'

// Button — the app's primary action element with theme-bound variants and a
// small size scale. Centralises the className strings that were previously
// copy-pasted across ~15 panels so accent/danger styling and sizing stay
// consistent and re-theme cleanly.
export type ButtonVariant = 'primary' | 'secondary' | 'danger'
export type ButtonSize = 'sm' | 'md' | 'lg'

const VARIANTS: Record<ButtonVariant, string> = {
  primary: 'bg-[var(--color-accent)] font-medium text-[var(--color-on-accent)] hover:opacity-90',
  secondary: 'border border-[var(--color-border)] hover:bg-[var(--color-surface-2)]',
  danger:
    'border border-[var(--color-danger)]/50 text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10',
}

const SIZES: Record<ButtonSize, string> = {
  sm: 'rounded px-2.5 py-1 text-xs',
  md: 'rounded px-3 py-1.5 text-sm',
  lg: 'rounded-lg px-4 py-2 text-sm',
}

interface Props extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant
  size?: ButtonSize
}

export function Button({ variant = 'primary', size = 'md', className = '', ...props }: Props) {
  return (
    <button
      className={`inline-flex items-center justify-center gap-1.5 ${SIZES[size]} ${VARIANTS[variant]} transition disabled:opacity-50 ${className}`}
      {...props}
    />
  )
}

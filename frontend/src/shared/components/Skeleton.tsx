import type { LucideIcon } from 'lucide-react'

// Skeleton — a single pulsing placeholder bar. Compose several of them to sketch
// the shape of content that is still loading, so the layout does not jump when
// the real data arrives.
export function Skeleton({ className = '' }: { className?: string }) {
  return (
    <div aria-hidden className={`animate-pulse rounded bg-[var(--color-surface-2)] ${className}`} />
  )
}

// LoadingState — the centered spinner + label placeholder shown while a panel is
// fetching its first page of data. It mirrors EmptyState's geometry so swapping
// one for the other keeps the layout stable.
export function LoadingState({
  label,
  icon: Icon,
  className = '',
}: {
  label: string
  icon?: LucideIcon
  className?: string
}) {
  return (
    <div
      role="status"
      data-testid="loading-state"
      className={`flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)] ${className}`}
    >
      {Icon ? (
        <Icon size={28} strokeWidth={1.5} className="opacity-40" />
      ) : (
        <span className="h-6 w-6 animate-spin rounded-full border-2 border-[var(--color-border)] border-t-[var(--color-accent)]" />
      )}
      <p>{label}</p>
    </div>
  )
}

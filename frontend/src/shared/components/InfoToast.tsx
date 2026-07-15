import { useEffect } from 'react'
import { Info, X } from 'lucide-react'

interface Props {
  message: string
  onDismiss: () => void
  // Auto-dismiss after this many ms (0 = never). Default 6s.
  ttl?: number
}

// InfoToast is the neutral, informational sibling of ErrorToast: a floating pill
// pinned to the bottom-right for non-error notices (e.g. "this global change
// affects every workspace"). Same placement/shape as ErrorToast but accent-toned,
// so an advisory never reads as a failure. Stacked slightly higher so an error
// toast (bottom-4) and an info toast do not overlap when both are shown.
export function InfoToast({ message, onDismiss, ttl = 6000 }: Props) {
  useEffect(() => {
    if (!message || !ttl) return
    const t = setTimeout(onDismiss, ttl)
    return () => clearTimeout(t)
  }, [message, ttl, onDismiss])

  if (!message) return null
  return (
    <div className="pointer-events-none fixed bottom-20 right-4 z-50 flex justify-end">
      <div
        role="status"
        className="pointer-events-auto flex max-w-md items-start gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_12%,var(--color-surface))] px-3 py-2 text-sm text-[var(--color-text)] shadow-[var(--shadow-md)]"
      >
        <Info size={16} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
        <span className="min-w-0 flex-1 break-words">{message}</span>
        <button
          onClick={onDismiss}
          title="Kapat"
          className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:bg-[color-mix(in_srgb,var(--color-accent)_20%,transparent)]"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  )
}

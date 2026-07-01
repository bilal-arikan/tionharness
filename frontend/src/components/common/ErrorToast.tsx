import { useEffect } from 'react'
import { AlertTriangle, X } from 'lucide-react'

interface Props {
  message: string
  onDismiss: () => void
  // Auto-dismiss after this many ms (0 = never). Default 8s.
  ttl?: number
}

// ErrorToast is the single, app-wide error surface: a floating pill pinned to the
// bottom-right. It replaces the per-view header error span so screens that no
// longer render the top header (the ones whose sidebar now reaches the top) still
// surface errors the same way everywhere.
export function ErrorToast({ message, onDismiss, ttl = 8000 }: Props) {
  useEffect(() => {
    if (!message || !ttl) return
    const t = setTimeout(onDismiss, ttl)
    return () => clearTimeout(t)
  }, [message, ttl, onDismiss])

  if (!message) return null
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex justify-end">
      <div
        role="alert"
        className="pointer-events-auto flex max-w-md items-start gap-2 rounded-lg border border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_15%,var(--color-surface))] px-3 py-2 text-sm text-[var(--color-danger)] shadow-[var(--shadow-md)]"
      >
        <AlertTriangle size={16} className="mt-0.5 shrink-0" />
        <span className="min-w-0 flex-1 break-words">{message}</span>
        <button
          onClick={onDismiss}
          title="Kapat"
          className="shrink-0 rounded p-0.5 text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_20%,transparent)]"
        >
          <X size={14} />
        </button>
      </div>
    </div>
  )
}

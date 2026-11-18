import { LoadingState } from '@/shared/components'
import { storedSessionDetailWidth } from './sessionDetailWidth'

// SessionDetailFallback stands in for the session detail panel while its lazy
// chunk loads. It draws the panel's own frame (same width, border and surface)
// so the loading message is centred inside the panel instead of sizing a bare
// container to its text and spilling past the window edge.
export function SessionDetailFallback({ label }: { label: string }) {
  return (
    <aside
      data-testid="session-detail-fallback"
      style={{ width: storedSessionDetailWidth() }}
      className="th-col flex h-full shrink-0 flex-col items-center overflow-hidden border-l border-[var(--color-border)] bg-[var(--color-surface)] max-md:!w-[85vw] max-md:!max-w-sm"
    >
      <LoadingState label={label} className="w-full min-w-0 break-words" />
    </aside>
  )
}

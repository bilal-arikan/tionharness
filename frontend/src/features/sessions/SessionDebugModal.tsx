import { Bug, X } from 'lucide-react'
import { ModalOverlay, PaneHeader } from '@/shared/components'
import { SessionDebugCard } from './SessionDebugCard'

interface Props {
  sessionId: string
  // Session title, shown in the modal header alongside the "Debug" label.
  title?: string
  // agentId → display name, forwarded to the workflow visualizations for lane/node
  // labels.
  agentNames?: Record<string, string>
  onClose: () => void
}

// SessionDebugModal is the dedicated Debug / observability panel — the debug
// journal (timings, tokens, per-tool latency/errors, anomalies, raw event log)
// plus the workflow visualizations, lifted out of the session-info inspector into
// its own overlay opened from the chat header's "Debug" button. It reuses
// SessionDebugCard in alwaysOpen mode (no inner fold; the modal supplies chrome).
export function SessionDebugModal({ sessionId, title, agentNames, onClose }: Props) {
  return (
    <ModalOverlay onClose={onClose} padding="p-6">
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Oturum debug"
        data-testid="session-debug-modal"
        className="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-[var(--shadow-lg)]"
        onClick={(e) => e.stopPropagation()}
      >
        <PaneHeader
          titleSlot={
            <>
              <Bug size={16} className="shrink-0 text-[var(--color-text-dim)]" />
              <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">
                Debug / Gözlemlenebilirlik{title ? ` — ${title}` : ''}
              </h2>
            </>
          }
          right={
            <button
              onClick={onClose}
              aria-label="Kapat"
              className="shrink-0 rounded-md p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <X size={16} />
            </button>
          }
        />

        <div className="min-h-0 flex-1 overflow-y-auto p-5">
          <SessionDebugCard sessionId={sessionId} agentNames={agentNames} alwaysOpen />
        </div>
      </div>
    </ModalOverlay>
  )
}

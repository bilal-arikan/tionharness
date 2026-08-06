import { useCallback, useEffect, useState } from 'react'
import { Network, X } from 'lucide-react'
import { api } from '@/api'
import { ModalOverlay } from '@/shared/components/ModalOverlay'
import type { SessionInfo } from '@/types'
import { CoordinatorSection } from './CoordinatorSection'

interface Props {
  sessionId: string
  onClose: () => void
  onError: (msg: string) => void
  // Navigate to another session (worker roster rows / coordinator back-link).
  onSelectSession?: (id: string) => void
  // Open the Skills screen on a coordinator-workflow slug.
  onOpenSkill?: (slug: string) => void
  // Bumped by the parent when the conversation changes, so the worker roster
  // refreshes as notifications land.
  refreshKey?: number
}

// CoordinatorPanel is the right-anchored drawer opened by the chat header's
// "Coord" button. It hosts the coordination UI (CoordinatorSection) — coordinator
// mode toggle, workflow picker, live worker roster and the coordinator tree —
// which used to live inside the session detail ("Oturum bilgisi") panel. Moving it
// here gives coordination its own dedicated affordance instead of one card buried
// in the detail inspector.
export function CoordinatorPanel({
  sessionId,
  onClose,
  onError,
  onSelectSession,
  onOpenSkill,
  refreshKey,
}: Props) {
  const [info, setInfo] = useState<SessionInfo | null>(null)
  const [loading, setLoading] = useState(false)
  // Local refetch nonce: bumped after a role/workflow toggle so the panel reflects
  // the new coordination state without touching the parent's refreshKey.
  const [localRefresh, setLocalRefresh] = useState(0)

  const load = useCallback(() => {
    let alive = true
    setLoading(true)
    api
      .sessionInfo(sessionId)
      .then((d) => alive && setInfo(d))
      .catch((e) => alive && onError((e as Error).message))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [sessionId, onError])

  useEffect(() => load(), [load, refreshKey, localRefresh])

  return (
    <ModalOverlay onClose={onClose} padding="p-0" className="!justify-end">
      <div
        className="flex h-full w-full max-w-[420px] flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)]"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <header className="flex items-center gap-2 border-b border-[var(--color-border)] px-3 py-2">
          <Network size={15} className="shrink-0 text-[var(--color-text-dim)]" />
          <span className="truncate text-sm font-medium">Koordinasyon</span>
          <button
            type="button"
            onClick={onClose}
            title="Kapat"
            className="ml-auto text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
          >
            <X size={15} />
          </button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-4">
          {loading && !info ? (
            <p className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</p>
          ) : !info ? (
            <p className="text-sm text-[var(--color-text-dim)]">Bilgi yok.</p>
          ) : (
            <CoordinatorSection
              sessionId={sessionId}
              role={info.role}
              coordinatorMode={info.coordinatorMode}
              coordinatorDepth={info.coordinatorDepth}
              workflow={info.coordinatorWorkflow}
              coordinatorSessionId={info.coordinatorSessionId}
              stallHalted={info.coordinatorStallHalted}
              refreshKey={(refreshKey ?? 0) + localRefresh}
              onError={onError}
              onRoleChanged={() => setLocalRefresh((n) => n + 1)}
              onSelectSession={onSelectSession}
              onOpenSkill={onOpenSkill}
            />
          )}
        </div>
      </div>
    </ModalOverlay>
  )
}

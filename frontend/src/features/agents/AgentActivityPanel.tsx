import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type MouseEvent as ReactMouseEvent,
} from 'react'
import {
  MessageSquare,
  LayoutGrid,
  GitBranch,
  Clock,
  Activity,
  RefreshCw,
  ChevronRight,
  X,
  type LucideIcon,
} from 'lucide-react'
import type { Execution } from '@/types'
import { api } from '@/api'
import { relativeTime } from '@/shared/lib/time'
import { useVisiblePoll } from '@/shared/hooks/useVisiblePoll'

interface Props {
  // The agent whose activity to show; null clears the panel.
  agentId: string | null
  onError: (msg: string) => void
  // Jump to the Activity (executions) view with this run pre-selected.
  onOpenExecution?: (sessionId: string) => void
  // Collapse the panel (matches the chat SessionDetailPanel close affordance).
  onClose?: () => void
}

// Per-kind display metadata mirrors the unified executions feed so an agent's
// activity rows carry the same icon + label vocabulary as the Activity screen.
const KIND_META: Record<string, { label: string; icon: LucideIcon }> = {
  chat: { label: 'Sohbet', icon: MessageSquare },
  task: { label: 'Görev', icon: LayoutGrid },
  flow: { label: 'Akış', icon: GitBranch },
  schedule: { label: 'Zamanlama', icon: Clock },
}

// Backstop refresh; the panel is visibility-gated (see useVisiblePoll).
const POLL_MS = 10000

function kindMeta(kind: string) {
  return KIND_META[kind] ?? { label: kind || 'Diğer', icon: Activity }
}

// AgentActivityPanel is the right-hand rail on the Ajanlar screen: a compact,
// live-polled feed of the selected agent's executions (chat / task / flow /
// schedule). It reuses GET /api/executions and filters by agentId,
// so no backend work is needed — every run path already funnels into a Session
// tagged with its owner agent. Clicking a row opens the full transcript on the
// Activity screen.
export function AgentActivityPanel({ agentId, onError, onOpenExecution, onClose }: Props) {
  const [items, setItems] = useState<Execution[]>([])
  const [loading, setLoading] = useState(false)

  // Resizable width (persisted, clamped). 320px == the old w-80. Drag the handle
  // on the panel's LEFT edge: moving it left widens the panel.
  const [width, setWidth] = useState(() => {
    const v = Number(localStorage.getItem('tionswarm.agentActivityWidth'))
    return v >= 240 && v <= 720 ? v : 320
  })
  useEffect(() => {
    localStorage.setItem('tionswarm.agentActivityWidth', String(width))
  }, [width])

  const startResize = useCallback(
    (e: ReactMouseEvent) => {
      e.preventDefault()
      const startX = e.clientX
      const startW = width
      const onMove = (ev: MouseEvent) =>
        setWidth(Math.min(720, Math.max(240, startW + (startX - ev.clientX))))
      const onUp = () => {
        document.removeEventListener('mousemove', onMove)
        document.removeEventListener('mouseup', onUp)
        document.body.style.cursor = ''
        document.body.style.userSelect = ''
      }
      document.addEventListener('mousemove', onMove)
      document.addEventListener('mouseup', onUp)
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
    },
    [width],
  )

  // Pull the unified feed and keep only this agent's rows (newest-updated first,
  // matching the backend's sort).
  const refresh = useCallback(() => {
    if (!agentId) return Promise.resolve()
    return api
      .listExecutions()
      .then((all) => setItems(all.filter((e) => e.agentId === agentId)))
      .catch((e) => onError((e as Error).message))
  }, [agentId, onError])

  // Initial load on agent change.
  useEffect(() => {
    if (!agentId) {
      setItems([])
      return
    }
    let alive = true
    setLoading(true)
    refresh().finally(() => {
      if (alive) setLoading(false)
    })
    return () => {
      alive = false
    }
  }, [agentId, refresh])

  // Backstop refresh, paused while this window is hidden.
  useVisiblePoll(refresh, POLL_MS, [refresh], Boolean(agentId))

  const runningCount = useMemo(() => items.filter((i) => i.running).length, [items])

  return (
    <aside
      style={{ width }}
      className="relative flex shrink-0 flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)] max-md:!w-full max-md:border-l-0 max-md:border-t"
    >
      {/* Drag handle on the left edge — widen the panel by dragging left. */}
      <div
        onMouseDown={startResize}
        title="Sürükleyerek genişlet"
        className="absolute left-0 top-0 z-10 h-full w-1 cursor-col-resize transition hover:bg-[color-mix(in_srgb,var(--color-accent)_50%,transparent)]"
      />
      <div className="flex items-center justify-between px-4 pt-4 pb-2">
        <span className="flex items-center gap-1.5 text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          <Activity size={13} /> Aktivite
          {runningCount > 0 && (
            <span className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] font-semibold text-[var(--color-accent)]">
              {runningCount} çalışıyor
            </span>
          )}
        </span>
        <div className="flex items-center gap-1">
          <button
            onClick={refresh}
            title="Yenile"
            className="text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          >
            <RefreshCw size={14} />
          </button>
          {onClose && (
            <button
              onClick={onClose}
              title="Aktivite panelini kapat"
              aria-label="Aktivite panelini kapat"
              className="text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
            >
              <X size={15} />
            </button>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-y-auto px-2 pb-2">
        {items.map((it) => {
          const meta = kindMeta(it.kind)
          const Icon = meta.icon
          const clickable = !!onOpenExecution
          return (
            <button
              key={it.sessionId}
              onClick={() => onOpenExecution?.(it.sessionId)}
              disabled={!clickable}
              className={`group mb-0.5 flex w-full items-start gap-2 rounded-lg px-3 py-2 text-left text-sm transition ${
                clickable
                  ? 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
                  : 'cursor-default text-[var(--color-text-dim)]'
              }`}
            >
              <span className="mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-2)]">
                <Icon size={12} />
              </span>
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="flex items-center gap-1.5">
                  {it.running ? (
                    <span className="relative flex h-2 w-2 shrink-0" title="Çalışıyor">
                      <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-[var(--color-success)] opacity-75" />
                      <span className="relative inline-flex h-2 w-2 rounded-full bg-[var(--color-success)]" />
                    </span>
                  ) : (
                    it.unread && (
                      <span
                        className="h-2 w-2 shrink-0 rounded-full bg-[var(--color-accent)]"
                        title="Okunmadı"
                      />
                    )
                  )}
                  <span
                    className={`min-w-0 flex-1 truncate ${
                      it.unread || it.running ? 'font-semibold text-[var(--color-text)]' : ''
                    }`}
                  >
                    {it.title || meta.label}
                  </span>
                </span>
                <span className="flex items-center gap-1.5 text-[10px] opacity-70">
                  <span>{meta.label}</span>
                  <span>· {relativeTime(it.updatedAt)}</span>
                  {it.lastStatus === 'success' && (
                    <span className="text-[var(--color-success)]">· başarılı</span>
                  )}
                  {it.lastStatus === 'failure' && (
                    <span className="text-[var(--color-danger)]">· hata</span>
                  )}
                </span>
              </span>
              {clickable && (
                <ChevronRight
                  size={14}
                  className="mt-0.5 shrink-0 opacity-0 transition group-hover:opacity-60"
                />
              )}
            </button>
          )
        })}
        {!loading && items.length === 0 && (
          <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
            {agentId
              ? 'Bu ajan için henüz aktivite yok. Bir sohbet, görev, akış veya zamanlama çalıştığında burada belirir.'
              : 'Aktiviteyi görmek için bir ajan seç.'}
          </p>
        )}
      </div>
    </aside>
  )
}

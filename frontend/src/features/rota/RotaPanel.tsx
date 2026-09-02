// Rota (trajectory) view — F0, projection only (_Docs/77 brief §11 F0).
// Workspace root zoom level: every recently active session is a lane on a
// time axis (git-graph metaphor), workers hang under their coordinator with
// spawn/report edges, automation fires and stall halts mark the lanes, armed
// schedules sit in the future strip. Data comes solely from the lane store
// (workspace stream + REST seed); nothing here writes.
import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Waypoints } from 'lucide-react'
import { api } from '@/api'
import { EmptyState } from '@/shared/components'
import { ViewPanel } from '@/features/view/ViewPanel'
import { useLanes, connectLanes, seedLanes, resetLanes } from '@/shared/lib/laneStore'
import { seedLiveness, seedSessions, seedTrajectories } from '@/shared/lib/laneReducer'
import { useServerNow } from './useServerNow'
import { layoutRota } from './rotaLayout'
import { RotaCanvas, type RotaSelection } from './RotaCanvas'
import { RotaActivity } from './RotaActivity'
import { RotaToolbar, type IdleCutoff } from './RotaToolbar'

interface Props {
  workspaceId: string
  onError: (msg: string) => void
  onOpenSession?: (sessionId: string) => void
  onOpenFlowRun?: (flowId: string) => void
}

// Sessions the seed pulls: the most recently active ones; older lanes arrive
// through the stream only when they change again.
const SEED_LIMIT = 200

export function RotaPanel({ workspaceId, onError, onOpenSession, onOpenFlowRun }: Props) {
  const lanes = useLanes()
  const now = useServerNow(5000)
  const [selected, setSelected] = useState<RotaSelection | null>(null)
  const [cutoff, setCutoff] = useState<IdleCutoff>(21600)
  const seeding = useRef(false)

  const seed = useCallback(async () => {
    if (seeding.current) return
    seeding.current = true
    try {
      const [page, live, trajectories] = await Promise.all([
        api.listSessions({ limit: SEED_LIMIT, sort: 'updated_desc', state: 'active' }),
        api.workspaceLiveness(),
        api.listTrajectories({ limit: SEED_LIMIT }),
      ])
      seedLanes((s) =>
        seedTrajectories(seedLiveness(seedSessions(s, page.items), live), trajectories),
      )
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      seeding.current = false
    }
  }, [onError])

  // Per workspace: drop the old picture, open the stream, then seed over REST.
  useEffect(() => {
    resetLanes()
    const release = connectLanes()
    void seed()
    return release
  }, [workspaceId, seed])

  useEffect(() => {
    if (lanes.stale) void seed()
  }, [lanes.stale, seed])

  // Canvas width follows the scroll container.
  const hostRef = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(960)
  useLayoutEffect(() => {
    const el = hostRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w) setWidth(Math.floor(w))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const layout = layoutRota(lanes, { now, idleCutoffSec: cutoff })
  const loading = lanes.revision === 0 || lanes.stale

  // ↑/↓ walk the lanes; Enter opens the selected session; Esc clears.
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (layout.rows.length === 0) return
    if (e.key === 'Escape') {
      setSelected(null)
      return
    }
    if (e.key === 'Enter' && selected?.kind === 'session') {
      onOpenSession?.(selected.id)
      return
    }
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    const idx = selected ? layout.rows.findIndex((r) => r.id === selected.id) : -1
    const next =
      e.key === 'ArrowDown' ? Math.min(layout.rows.length - 1, idx + 1) : Math.max(0, idx - 1)
    setSelected({ kind: 'session', id: layout.rows[next].id })
  }

  return (
    <div className="flex h-full flex-1 flex-col overflow-hidden" tabIndex={0} onKeyDown={onKeyDown}>
      <header className="flex flex-wrap items-center gap-3 border-b border-[var(--color-border)] px-4 py-2 text-xs">
        <Waypoints size={16} className="opacity-70" />
        <span className="font-medium">Rota</span>
        <span
          className={`h-2 w-2 rounded-full ${lanes.connected ? 'bg-emerald-500' : 'bg-amber-500'}`}
          title={lanes.connected ? 'Canlı akış bağlı' : 'Akış bağlanıyor…'}
        />
        <span className="text-[var(--color-text-dim)]">
          {layout.rows.filter((r) => r.depth === 0).length} şerit · {lanes.sessions.size} oturum ·{' '}
          {lanes.trajectories.size} rota · {lanes.flowRuns.size} akış koşusu · {lanes.fires.length}{' '}
          tetik
        </span>
        {lanes.capacity && (
          <span className="text-[var(--color-text-dim)]">
            spawn {lanes.capacity.spawnActive}/{lanes.capacity.spawnMax} · kuyruk{' '}
            {lanes.capacity.queueDepth}/{lanes.capacity.queueMax}
            {lanes.capacity.autonomyPaused ? ' · otonomi duraklatıldı' : ''}
          </span>
        )}
        <RotaToolbar cutoff={cutoff} onCutoff={setCutoff} />
        <span className="ml-auto text-[var(--color-text-dim)]" title="Akış seq / depo revizyonu">
          #{lanes.head} · r{lanes.revision}
        </span>
      </header>
      <div className="flex min-h-0 flex-1">
        <div ref={hostRef} className="min-w-0 flex-1 overflow-auto">
          {layout.rows.length === 0 ? (
            <EmptyState
              icon={Waypoints}
              title={loading ? 'Şeritler yükleniyor…' : 'Bu pencerede şerit yok'}
            >
              Bir oturum başladığında burada bir şerit olarak görünür; süzgeci genişletmek için
              üstteki pencereyi değiştir.
            </EmptyState>
          ) : (
            <RotaCanvas
              layout={layout}
              width={width}
              selected={selected}
              onSelect={setSelected}
              onOpenSession={onOpenSession}
              onOpenFlowRun={onOpenFlowRun}
            />
          )}
        </div>
        <aside className="hidden w-80 shrink-0 flex-col overflow-auto border-l border-[var(--color-border)] md:flex">
          {selected ? (
            <div className="flex min-h-0 flex-1 flex-col">
              <ViewPanel
                key={`${selected.kind}:${selected.id}`}
                target={{ kind: selected.kind, id: selected.id }}
                embedded
                hideHandles
                fillHeight
                onClose={() => setSelected(null)}
              />
            </div>
          ) : (
            <RotaActivity lanes={lanes} />
          )}
        </aside>
      </div>
    </div>
  )
}

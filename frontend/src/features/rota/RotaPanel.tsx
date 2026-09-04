// Rota (trajectory) view — F0, projection only (_Docs/77 brief §11 F0).
// Workspace root zoom level: every recently active session is a lane on a
// time axis (git-graph metaphor), workers hang under their coordinator with
// spawn/report edges, automation fires and stall halts mark the lanes, armed
// schedules sit in the future strip. Data comes solely from the lane store
// (workspace stream + REST seed); nothing here writes.
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { MessageSquare, Waypoints } from 'lucide-react'
import { api } from '@/api'
import { EmptyState } from '@/shared/components'
import { ViewPanel } from '@/features/view/ViewPanel'
import { useLanes, connectLanes, seedLanes, resetLanes } from '@/shared/lib/laneStore'
import { trajectoryForRoot } from '@/shared/lib/laneModel'
import { seedLiveness, seedSessions, seedTrajectories } from '@/shared/lib/laneReducer'
import { useServerNow } from './useServerNow'
import { layoutRota } from './rotaLayout'
import { RotaCanvas, type RotaSelection } from './RotaCanvas'
import { RotaActivity } from './RotaActivity'
import { RotaToolbar, type IdleCutoff } from './RotaToolbar'
import { RotaChipFilter } from './RotaChipFilter'
import { RotaZoomControl } from './RotaZoomControl'
import { MIN_ZOOM, anchoredScrollLeft, clampZoom, stepZoom } from './rotaZoom'
import { ROTA_LABEL_W } from './RotaCanvas'
import { laneChipCounts, laneChipFilter } from './rotaChips'
import { useRotaChips } from './useRotaChips'
import { RotaTrajectoryView } from './RotaTrajectoryView'

interface Props {
  workspaceId: string
  onError: (msg: string) => void
  onOpenSession?: (sessionId: string) => void
  onOpenFlowRun?: (flowId: string) => void
  // Zoomed trajectory (deep link #/w/WS/rota/RTA12); null = workspace lanes.
  trajectoryId: string | null
  onTrajectory: (id: string | null) => void
}

// Sessions the seed pulls: the most recently active ones; older lanes arrive
// through the stream only when they change again.
const SEED_LIMIT = 200

export function RotaPanel({
  workspaceId,
  onError,
  onOpenSession,
  onOpenFlowRun,
  trajectoryId,
  onTrajectory,
}: Props) {
  const lanes = useLanes()
  const now = useServerNow(5000)
  const [selected, setSelected] = useState<RotaSelection | null>(null)
  const [cutoff, setCutoff] = useState<IdleCutoff>(21600)
  // Dead air in the time axis is collapsed by default: an idle night otherwise
  // squashes the minutes that carry work into a few pixels.
  const [collapseGaps, setCollapseGaps] = useState(true)
  // Duration is spent on a log axis by default: over a multi-day window most
  // bars otherwise round to a couple of pixels next to one very long lane.
  const [normalizeBars, setNormalizeBars] = useState(true)
  // Time-axis zoom; 1 fits the panel, above that the canvas host scrolls.
  const [zoom, setZoom] = useState(MIN_ZOOM)
  const zoomRef = useRef(zoom)
  zoomRef.current = zoom
  // Scroll position to restore once the wider canvas has been laid out; set by
  // the wheel handler, applied in the layout effect below.
  const pendingScroll = useRef<number | null>(null)
  const chips = useRotaChips()
  const seeding = useRef(false)
  const hostRef = useRef<HTMLDivElement>(null)

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

  // Ctrl/⌘ + wheel zooms the time axis, keeping the instant under the cursor
  // fixed. Registered natively (not via onWheel) because the listener has to be
  // non-passive to preventDefault the browser's own page zoom.
  useEffect(() => {
    const el = hostRef.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      if (!e.ctrlKey && !e.metaKey) return
      e.preventDefault()
      // Read the current zoom from a ref rather than inside the state updater:
      // an updater must stay pure (React may call it twice), and the scroll
      // correction below is a side effect that has to run exactly once.
      const prev = zoomRef.current
      const next = clampZoom(prev * (e.deltaY < 0 ? 1.15 : 1 / 1.15))
      if (next === prev) return
      const anchor = e.clientX - el.getBoundingClientRect().left
      const left = anchoredScrollLeft(el.scrollLeft, anchor, prev, next, ROTA_LABEL_W)
      pendingScroll.current = left
      setZoom(next)
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  // Apply the anchored scroll after layout: at this point the canvas has been
  // committed at the new zoom, so scrollLeft is no longer clamped to the old
  // (narrower) scrollWidth.
  useLayoutEffect(() => {
    const el = hostRef.current
    const left = pendingScroll.current
    if (!el || left === null) return
    pendingScroll.current = null
    el.scrollLeft = left
  }, [zoom])

  // Canvas width follows the scroll container.
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

  const chipCounts = useMemo(() => laneChipCounts(lanes), [lanes])
  const laneFilter = useMemo(() => laneChipFilter(lanes, chips.chipSet), [lanes, chips.chipSet])
  const layout = layoutRota(lanes, { now, idleCutoffSec: cutoff, laneFilter })
  const loading = lanes.revision === 0 || lanes.stale

  // The selected session's own trajectory (for the "◈ Rotayı aç" shortcut).
  const selectedLane = selected?.kind === 'session' ? lanes.sessions.get(selected.id) : undefined
  const selectedRoot = selectedLane ? selectedLane.rootSessionId || selectedLane.id : undefined
  const selectedTrajectory = (() => {
    if (selected?.kind !== 'session') return undefined
    return trajectoryForRoot(lanes, selectedRoot ?? selected.id)
  })()
  // A worker lane's own thread is a fragment of the coordinator's run, so the
  // side panel offers both: this lane, and the root session it belongs to.
  const rootIsOther = !!selectedRoot && !!selectedLane && selectedRoot !== selectedLane.id
  const rootTitle = rootIsOther ? lanes.sessions.get(selectedRoot)?.title : undefined

  // ↑/↓ walk the lanes; Enter opens the selected session; Esc clears (or
  // leaves the trajectory zoom).
  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      if (selected) setSelected(null)
      else if (trajectoryId) onTrajectory(null)
      return
    }
    if (e.key === '+' || e.key === '=') {
      e.preventDefault()
      setZoom((z) => stepZoom(z, 1))
      return
    }
    if (e.key === '-' || e.key === '_') {
      e.preventDefault()
      setZoom((z) => stepZoom(z, -1))
      return
    }
    if (e.key === '0') {
      e.preventDefault()
      setZoom(MIN_ZOOM)
      return
    }
    if (trajectoryId || layout.rows.length === 0) return
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
        <RotaZoomControl zoom={zoom} onZoom={setZoom} />
        <RotaToolbar
          cutoff={cutoff}
          onCutoff={setCutoff}
          collapseGaps={collapseGaps}
          onCollapseGaps={setCollapseGaps}
          normalizeBars={normalizeBars}
          onNormalizeBars={setNormalizeBars}
        />
        <span className="ml-auto text-[var(--color-text-dim)]" title="Akış seq / depo revizyonu">
          #{lanes.head} · r{lanes.revision}
        </span>
      </header>
      {!trajectoryId && (
        <RotaChipFilter
          chipSet={chips.chipSet}
          counts={chipCounts}
          onClickChip={chips.clickChip}
          onReset={chips.reset}
          offCount={chips.chipsOff.length}
        />
      )}
      <div className="flex min-h-0 flex-1">
        <div ref={hostRef} className="min-w-0 flex-1 overflow-auto">
          {layout.rows.length === 0 && !trajectoryId ? (
            <EmptyState
              icon={Waypoints}
              title={loading ? 'Şeritler yükleniyor…' : 'Bu pencerede şerit yok'}
            >
              Bir oturum başladığında burada bir şerit olarak görünür; süzgeci genişletmek için
              üstteki pencereyi değiştir.
            </EmptyState>
          ) : trajectoryId ? (
            <RotaTrajectoryView
              trajectoryId={trajectoryId}
              width={width}
              selected={selected}
              onSelect={setSelected}
              onOpenSession={onOpenSession}
              onOpenFlowRun={onOpenFlowRun}
              onBack={() => {
                setSelected(null)
                onTrajectory(null)
              }}
            />
          ) : (
            <RotaCanvas
              layout={layout}
              width={width}
              selected={selected}
              onSelect={setSelected}
              onOpenSession={onOpenSession}
              onOpenFlowRun={onOpenFlowRun}
              collapseGaps={collapseGaps}
              normalizeBars={normalizeBars}
              zoom={zoom}
              onOpenTrajectory={(id) => {
                setSelected(null)
                onTrajectory(id)
              }}
            />
          )}
        </div>
        <aside className="hidden w-80 shrink-0 flex-col overflow-auto border-l border-[var(--color-border)] md:flex">
          {selected ? (
            <div className="flex min-h-0 flex-1 flex-col">
              {selected.kind === 'session' && (
                <div className="flex flex-wrap items-center gap-1 border-b border-[var(--color-border)] px-3 py-1.5">
                  <button
                    type="button"
                    onClick={() => onOpenSession?.(selected.id)}
                    className="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs text-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
                    title={`${selected.id} oturumunu Sohbet'te aç`}
                  >
                    <MessageSquare size={13} />
                    Sohbette aç
                  </button>
                  {rootIsOther && (
                    <button
                      type="button"
                      onClick={() => onOpenSession?.(selectedRoot)}
                      className="flex items-center gap-1.5 rounded px-1.5 py-0.5 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                      title={`Bu şeridin bağlı olduğu kök oturum: ${rootTitle || selectedRoot}`}
                    >
                      ↰ Asıl oturum
                    </button>
                  )}
                </div>
              )}
              {selectedTrajectory && selectedTrajectory.trajectoryId !== trajectoryId && (
                <button
                  type="button"
                  onClick={() => {
                    setSelected(null)
                    onTrajectory(selectedTrajectory.trajectoryId)
                  }}
                  className="flex items-center gap-1.5 border-b border-[var(--color-border)] px-3 py-1.5 text-left text-xs text-[var(--color-accent)] hover:bg-[var(--color-surface-2)]"
                  title={`${selectedTrajectory.trajectoryId} · rev ${selectedTrajectory.revision}`}
                >
                  ◈ Rotayı aç · {selectedTrajectory.templateRef || 'plansız'} ·{' '}
                  {selectedTrajectory.status}
                </button>
              )}
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
            <RotaActivity lanes={lanes} onOpenTrajectory={onTrajectory} />
          )}
        </aside>
      </div>
    </div>
  )
}

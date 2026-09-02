// Rota (trajectory) view — F0 skeleton (_Docs/77 R10). Renders the lane store
// as plain text lanes: one row per root session, its members indented, with
// the liveness state, the trajectory revision and the latest automation fires.
// No graph yet: this proves the stream → laneStore → panel path end to end so
// F0's canvas can replace the body without touching the data flow.
import { useCallback, useEffect, useRef } from 'react'
import { Waypoints } from 'lucide-react'
import { api } from '@/api'
import { EmptyState } from '@/shared/components'
import { useLanes, connectLanes, seedLanes, resetLanes } from '@/shared/lib/laneStore'
import { seedLiveness, seedSessions } from '@/shared/lib/laneReducer'
import { rootLanes } from '@/shared/lib/laneModel'
import { RotaLane } from './RotaLane'
import { RotaActivity } from './RotaActivity'

interface Props {
  workspaceId: string
  onError: (msg: string) => void
  onOpenSession?: (sessionId: string) => void
}

// Sessions the seed pulls: the most recently active ones; older lanes arrive
// through the stream only when they change again.
const SEED_LIMIT = 200

export function RotaPanel({ workspaceId, onError, onOpenSession }: Props) {
  const lanes = useLanes()
  // In-flight guard for the REST seed; a ref (not state) so the effects below
  // never set React state synchronously.
  const seeding = useRef(false)

  const seed = useCallback(async () => {
    if (seeding.current) return
    seeding.current = true
    try {
      const [page, live] = await Promise.all([
        api.listSessions({ limit: SEED_LIMIT, sort: 'updated_desc', state: 'active' }),
        api.workspaceLiveness(),
      ])
      seedLanes((s) => seedLiveness(seedSessions(s, page.items), live))
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      seeding.current = false
    }
  }, [onError])

  // Per workspace: drop the old picture, open the (workspace-scoped) stream,
  // then seed over REST. Stream first so events that land while the seed is in
  // flight are newer than the seed and survive it (reducer stale rule). The
  // cleanup releases the stream; the next workspace's run reopens it.
  useEffect(() => {
    resetLanes()
    const release = connectLanes()
    void seed()
    return release
  }, [workspaceId, seed])

  // Cursor reset: re-seed once; the reducer clears `stale` on the next seed.
  useEffect(() => {
    if (lanes.stale) void seed()
  }, [lanes.stale, seed])

  const roots = rootLanes(lanes)
  const loading = lanes.revision === 0 || lanes.stale

  return (
    <div className="flex h-full flex-1 flex-col overflow-hidden">
      <header className="flex items-center gap-3 border-b border-[var(--color-border)] px-4 py-2 text-xs">
        <Waypoints size={16} className="opacity-70" />
        <span className="font-medium">Rota</span>
        <span
          className={`h-2 w-2 rounded-full ${lanes.connected ? 'bg-emerald-500' : 'bg-amber-500'}`}
          title={lanes.connected ? 'Canlı akış bağlı' : 'Akış bağlanıyor…'}
        />
        <span className="text-[var(--color-text-dim)]">
          {roots.length} şerit · {lanes.sessions.size} oturum · {lanes.trajectories.size} rota ·{' '}
          {lanes.flowRuns.size} akış koşusu · {lanes.fires.length} tetik
        </span>
        {lanes.capacity && (
          <span className="ml-auto text-[var(--color-text-dim)]">
            spawn {lanes.capacity.spawnActive}/{lanes.capacity.spawnMax} · kuyruk{' '}
            {lanes.capacity.queueDepth}/{lanes.capacity.queueMax}
            {lanes.capacity.autonomyPaused ? ' · otonomi duraklatıldı' : ''}
          </span>
        )}
        <span className="text-[var(--color-text-dim)]" title="Akış seq / depo revizyonu">
          #{lanes.head} · r{lanes.revision}
        </span>
      </header>
      <div className="flex min-h-0 flex-1">
        <div className="min-w-0 flex-1 overflow-auto p-3">
          {roots.length === 0 ? (
            <EmptyState
              icon={Waypoints}
              title={loading ? 'Şeritler yükleniyor…' : 'Henüz şerit yok'}
            >
              Bir oturum başladığında burada bir şerit olarak görünür.
            </EmptyState>
          ) : (
            <ul className="flex flex-col gap-2">
              {roots.map((root) => (
                <RotaLane key={root.id} root={root} lanes={lanes} onOpenSession={onOpenSession} />
              ))}
            </ul>
          )}
        </div>
        <RotaActivity lanes={lanes} />
      </div>
    </div>
  )
}

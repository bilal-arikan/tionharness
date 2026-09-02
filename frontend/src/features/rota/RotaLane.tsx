// One lane of the Rota skeleton: a root session and the sessions under it.
import {
  laneMembers,
  trajectoryForRoot,
  type LaneSession,
  type LaneState,
} from '@/shared/lib/laneModel'
import { laneLiveLabel, laneOriginGlyph } from './rotaLabels'

interface Props {
  root: LaneSession
  lanes: LaneState
  onOpenSession?: (sessionId: string) => void
}

export function RotaLane({ root, lanes, onOpenSession }: Props) {
  const members = laneMembers(lanes, root.id)
  const traj = trajectoryForRoot(lanes, root.id)
  return (
    <li className="rounded border border-[var(--color-border)] bg-[var(--color-surface)] p-2 text-xs">
      <SessionRow s={root} onOpenSession={onOpenSession} />
      {traj && (
        <div className="mt-1 pl-5 text-[var(--color-text-dim)]">
          rota {traj.trajectoryId} · {traj.templateRef || 'plansız'} · {traj.status ?? '?'} · rev{' '}
          {traj.revision} · {traj.nodeCount} düğüm
        </div>
      )}
      {members.length > 0 && (
        <ul className="mt-1 flex flex-col gap-0.5 border-l border-[var(--color-border)] pl-3">
          {members.map((m) => (
            <li key={m.id}>
              <SessionRow s={m} onOpenSession={onOpenSession} />
            </li>
          ))}
        </ul>
      )}
    </li>
  )
}

function SessionRow({
  s,
  onOpenSession,
}: {
  s: LaneSession
  onOpenSession?: (id: string) => void
}) {
  const live = laneLiveLabel(s.live)
  return (
    <button
      type="button"
      className="flex w-full items-center gap-2 text-left hover:underline"
      onClick={() => onOpenSession?.(s.id)}
      title={`${s.id} · ${s.kind}${s.origin ? ` · köken ${s.origin.kind}` : ''}`}
    >
      <span className="w-4 text-center opacity-70">{laneOriginGlyph(s)}</span>
      <span className="truncate">{s.title || s.id}</span>
      <span className="text-[var(--color-text-dim)]">{s.kind}</span>
      {live && <span className="rounded bg-emerald-500/15 px-1 text-emerald-600">{live}</span>}
      {s.runState && !live && <span className="text-[var(--color-text-dim)]">{s.runState}</span>}
      {s.state === 'archived' && <span className="text-[var(--color-text-dim)]">🗄</span>}
    </button>
  )
}

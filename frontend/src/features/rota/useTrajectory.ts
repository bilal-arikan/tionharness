// useTrajectory loads one trajectory graph and keeps it fresh: the workspace
// stream's ws:trajectory summary (lane store) carries the revision, so a
// revision bump for this id triggers a REST re-read of the full graph — the
// event never carries nodes (_Docs/78 §5). The lane store connection is
// ref-counted, so a chat header strip and the Rota screen share one stream.
import { useEffect, useState } from 'react'
import { api } from '@/api'
import type { Session } from '@/types'
import type { Trajectory } from '@/types/trajectory'
import { connectLanes, useLanes } from '@/shared/lib/laneStore'
import { trajectoryForRoot } from '@/shared/lib/laneModel'

export interface TrajectoryHandle {
  trajectory: Trajectory | null
  loading: boolean
  error: string | null
  // Resolved trajectory id ("" while unknown / none).
  id: string
}

// Either the trajectory id or the root session id may be given; the root form
// resolves through the index (one per root session).
export function useTrajectory(sel: {
  id?: string | null
  rootSessionId?: string | null
}): TrajectoryHandle {
  const lanes = useLanes()
  const wantId = sel.id ?? ''
  const wantRoot = sel.rootSessionId ?? ''
  // Live revision from the stream (either addressing form).
  const live = wantId
    ? lanes.trajectories.get(wantId)
    : wantRoot
      ? trajectoryForRoot(lanes, wantRoot)
      : undefined
  const liveRev = live?.revision ?? 0
  const liveOp = live?.op ?? ''
  // The request key the answer below belongs to; a mismatch means "loading".
  const key = wantId || wantRoot ? `${wantId}|${wantRoot}|${liveRev}|${liveOp}` : ''
  const [state, setState] = useState<{ key: string; t: Trajectory | null; err: string | null }>({
    key: '',
    t: null,
    err: null,
  })

  useEffect(() => {
    if (!wantId && !wantRoot) return
    return connectLanes()
  }, [wantId, wantRoot])

  useEffect(() => {
    if (!key) return
    let cancelled = false
    const load = async () => {
      if (liveOp === 'delete') return { t: null, err: null }
      let id = wantId
      if (!id) {
        const rows = await api.listTrajectories({ root: wantRoot, limit: 1 })
        id = rows[0]?.id ?? ''
        if (!id) return { t: null, err: null }
      }
      return { t: await api.getTrajectory(id), err: null }
    }
    load()
      .then((r) => {
        if (!cancelled) setState({ key, ...r })
      })
      .catch((e) => {
        if (!cancelled) setState({ key, t: null, err: e instanceof Error ? e.message : String(e) })
      })
    return () => {
      cancelled = true
    }
    // key folds wantId / wantRoot / liveRev / liveOp: a new stream revision re-reads.
  }, [key, wantId, wantRoot, liveOp])

  const current = state.key === key
  return {
    trajectory: current ? state.t : key ? state.t : null,
    loading: !!key && !current,
    error: current ? state.err : null,
    id: state.t?.id ?? wantId,
  }
}

// rootOf resolves the tree root for any member (origin first, legacy field
// second, itself last) — same precedence as the lane reducer.
export function rootOf(s: Session): string {
  return s.origin?.rootSessionId || s.rootCoordinatorSessionId || s.id
}

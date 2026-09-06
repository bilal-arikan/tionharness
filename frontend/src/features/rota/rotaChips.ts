// Rota lane chips — the same multi-select filter the session sidebar puts above
// its list, applied to the lanes of the F0 canvas. Reuses the sidebar's chip
// vocabulary and predicate (sessionKindMeta) so a chip means exactly the same
// thing on both screens; only the shape adapter and the tree rule live here.
//
// Pure, no DOM: layoutRota takes a predicate, these functions build it.
import type { LaneSession, LaneState } from '@/shared/lib/laneModel'
import { laneMembers, rootLanes } from '@/shared/lib/laneModel'
import {
  ARCHIVED_CHIP,
  WORKER_CHIP,
  nextChipsOff,
  sessionChipKey,
  sessionLiveScope,
  sessionMatchesChips,
  type ChipClickMode,
} from '@/features/sessions/sessionKindMeta'

/** Separate from the sidebar's key: unticking "Worker" on Rota should not
 *  change what the Sohbet list shows, and the two screens filter different
 *  things (lanes vs rows). */
export const ROTA_CHIPS_OFF_KEY = 'tionharness.rotaChipsOff'

/** A lane is a worker when it hangs under a root; the canvas already indents
 *  these, and the sidebar's Worker chip covers the same idea. */
function isWorkerLane(s: LaneSession): boolean {
  return !!s.rootSessionId && s.rootSessionId !== s.id
}

/** The lane's own turn is streaming. `runState` is the header fact; `live` is
 *  the fresher liveness signal, so it wins when present. */
function isRunningLane(s: LaneSession): boolean {
  const state = s.live?.state ?? s.runState
  return state === 'running' || state === 'streaming'
}

/** Translate a lane into the shape `sessionMatchesChips` classifies. The lane
 *  store carries no `category`/`executionType`, so the origin kind stands in:
 *  a subagent lane is exactly one spawned with `origin.kind === 'subagent'`. */
function laneChipShape(
  state: LaneState,
  s: LaneSession,
): Parameters<typeof sessionMatchesChips>[0] {
  const liveWorkerCount = isWorkerLane(s)
    ? 0
    : laneMembers(state, s.id).filter((m) => !!m.live || m.runState === 'running').length
  return {
    kind: s.kind,
    executionType: s.origin?.kind === 'subagent' ? 'subagent' : undefined,
    isWorker: isWorkerLane(s),
    isArchived: s.state === 'archived',
    isRunning: isRunningLane(s),
    liveWorkerCount,
  }
}

/** Does this one lane survive the selection, ignoring its tree? */
export function laneMatchesChips(
  state: LaneState,
  s: LaneSession,
  selected: ReadonlySet<string>,
): boolean {
  return sessionMatchesChips(laneChipShape(state, s), selected)
}

/** The predicate layoutRota filters lanes with. A root is kept when it matches
 *  OR any of its members does: dropping a coordinator whose worker the chips
 *  asked for would orphan that worker and cut its spawn edge. Members are
 *  filtered on their own. */
export function laneChipFilter(
  state: LaneState,
  selected: ReadonlySet<string>,
): (s: LaneSession) => boolean {
  return (s) => {
    if (laneMatchesChips(state, s, selected)) return true
    if (isWorkerLane(s)) return false
    return laneMembers(state, s.id).some((m) => laneMatchesChips(state, m, selected))
  }
}

/** chipKey → how many lanes in the whole store carry it, for the chip badges.
 *  Counted before the selection is applied, so an unticked chip still says how
 *  much it is hiding (the sidebar's chip badges read the same way).
 *
 *  Counted from the classification directly rather than by probing
 *  `laneMatchesChips` one chip at a time: that predicate needs a lane's kind
 *  chip AND its scope chips all on at once, so a single-chip set would score
 *  every worker as zero. A lane answers each axis it belongs to, so one running
 *  worker chat bumps Sohbet, Worker and Çalışan. */
export function laneChipCounts(state: LaneState): Map<string, number> {
  const counts = new Map<string, number>()
  const bump = (key: string) => counts.set(key, (counts.get(key) ?? 0) + 1)
  for (const root of rootLanes(state)) {
    for (const s of [root, ...laneMembers(state, root.id)]) {
      const shape = laneChipShape(state, s)
      bump(sessionChipKey(shape))
      if (shape.isWorker) bump(WORKER_CHIP)
      if (shape.isArchived) bump(ARCHIVED_CHIP)
      const live = sessionLiveScope({
        isRunning: shape.isRunning ?? false,
        liveWorkerCount: shape.liveWorkerCount ?? 0,
      })
      if (live) bump(live)
    }
  }
  return counts
}

export { nextChipsOff, type ChipClickMode }

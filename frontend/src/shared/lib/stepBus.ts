// A payload-carrying pub/sub for live turn-activity frames (session_step) and
// turn-end signals, scoped per session id. It mirrors refreshSignals.ts, but
// where that store only bumps a numeric tick, this one delivers the actual
// TurnStep payload to subscribers.
//
// Why it exists: the app opens exactly ONE SSE feed (api.subscribeEvents in
// useAppEvents). Its session_step frames used to reach a single consumer — the
// chat's ghost-bubble layer (chat.applyAutoStep). Any other transcript view
// (ExecutionsPanel, and later the flow-run viewer) was live-blind: it only
// polled api.listMessages, so a still-running turn showed working dots but no
// tool steps until the turn persisted. This bus fans the same frames out to N
// subscribers without opening a second EventSource: useAppEvents publishes here,
// each transcript view subscribes for the session it is showing and folds the
// steps through the shared chatStreamAutoLive helpers.

import type { TurnStep } from '@/types'

type StepListener = (step: TurnStep) => void
type EndListener = () => void

// Per-session subscriber sets. Keyed by session id so a view only hears the
// frames for the session it is currently showing.
const stepListeners = new Map<string, Set<StepListener>>()
const endListeners = new Map<string, Set<EndListener>>()

// publishStep delivers one live turn-step frame to every subscriber watching
// `sid`. Called by useAppEvents.onStep for every session_step SSE frame.
export function publishStep(sid: string, step: TurnStep): void {
  stepListeners.get(sid)?.forEach((l) => l(step))
}

// publishTurnEnd signals that `sid`'s turn has ended (chat / spawn / worker /
// schedule completion). Subscribers drop their live ghost bubble and reload the
// authoritative persisted transcript. Called by useAppEvents' completion
// branches, unconditionally (not gated by the chat's active session) so a view
// showing a different session than the chat still hears it.
export function publishTurnEnd(sid: string): void {
  endListeners.get(sid)?.forEach((l) => l())
}

// subscribeStep registers `cb` for `sid`'s live step frames. Returns an
// unsubscribe function; the per-session set is pruned when it empties.
export function subscribeStep(sid: string, cb: StepListener): () => void {
  return add(stepListeners, sid, cb)
}

// subscribeTurnEnd registers `cb` for `sid`'s turn-end signal. Returns an
// unsubscribe function.
export function subscribeTurnEnd(sid: string, cb: EndListener): () => void {
  return add(endListeners, sid, cb)
}

function add<T>(map: Map<string, Set<T>>, sid: string, cb: T): () => void {
  let set = map.get(sid)
  if (!set) {
    set = new Set()
    map.set(sid, set)
  }
  set.add(cb)
  return () => {
    const s = map.get(sid)
    if (!s) return
    s.delete(cb)
    if (s.size === 0) map.delete(sid)
  }
}

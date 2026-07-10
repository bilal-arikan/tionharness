// Autonomous / other-window live turns and post-reload recovery.
//
// A synthetic "ghost" assistant bubble per session, grown from `session_step`
// bus frames. It lets a window that does NOT own the running turn — a scheduled/
// spawned/worker/wake turn, or the same chat turn viewed in another window —
// render thinking/tool steps live, exactly like the locally-streamed bubble.
// Replaced by the authoritative persisted message when the turn ends (the
// completion event reloads the transcript). Keyed by session because turns are
// detached and several can overlap.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import { api } from '@/api'
import type { InflightSnapshot, Message, TurnStep } from '@/types'
import { withAdded, withRemoved } from './chatStreamHelpers'
import type { AutoLiveEntry, RunHandle } from './chatStreamTypes'

export interface AutoLiveContext {
  activeSessionIdRef: RefObject<string | null>
  runsRef: RefObject<Map<string, RunHandle>>
  autoLiveRef: RefObject<Map<string, AutoLiveEntry>>
  setMessages: Dispatch<SetStateAction<Message[]>>
  setPendingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
}

// foldAutoStep folds one bus step into the active session's ghost bubble. It is
// a no-op when THIS window owns the turn (runsRef has it → the local per-request
// SSE already renders it, so the bus copy would duplicate), and only mutates the
// transcript for the session currently on screen (off-screen turns just raise the
// thinking indicator and reload their transcript on completion).
export function foldAutoStep(ctx: AutoLiveContext, sid: string, step: TurnStep): void {
  const { activeSessionIdRef, runsRef, autoLiveRef, setMessages, setPendingSessions } = ctx
  if (runsRef.current.has(sid)) return // this window owns the turn — avoid a duplicate ghost
  if (activeSessionIdRef.current !== sid) {
    setPendingSessions((p) => withAdded(p, sid))
    return
  }
  let entry = autoLiveRef.current.get(sid)
  if (!entry) {
    entry = { id: `live-auto-${sid}-${Date.now()}`, steps: [] }
    autoLiveRef.current.set(sid, entry)
    const bubble: Message = {
      id: entry.id,
      sessionId: sid,
      role: 'assistant',
      text: '',
      steps: '[]',
      createdAt: Math.floor(Date.now() / 1000),
    }
    setMessages((prev) => (prev.some((m) => m.id === entry!.id) ? prev : [...prev, bubble]))
  }
  // Merge streamed reasoning chunks (same id) into one growing thinking block;
  // a tombstone retracts a prior step; everything else appends.
  if (step.kind === 'tombstone') {
    entry.steps = entry.steps.filter((s) => s.id !== step.ref)
  } else if (step.kind === 'thinking' && step.id) {
    const idx = entry.steps.findIndex((s) => s.kind === 'thinking' && s.id === step.id)
    if (idx >= 0) {
      const merged = { ...entry.steps[idx], text: (entry.steps[idx].text || '') + (step.text || '') }
      entry.steps = entry.steps.map((s, k) => (k === idx ? merged : s))
    } else {
      entry.steps = [...entry.steps, step]
    }
  } else {
    entry.steps = [...entry.steps, step]
  }
  const json = JSON.stringify(entry.steps)
  const id = entry.id
  setMessages((prev) => prev.map((m) => (m.id === id ? { ...m, steps: json } : m)))
  setPendingSessions((p) => withRemoved(p, sid))
}

// clearAutoLiveEntry drops a session's ghost bubble + its accumulator once the
// turn has ended. The caller reloads the transcript right after, so the
// authoritative persisted message takes the ghost's place.
export function clearAutoLiveEntry(
  autoLiveRef: RefObject<Map<string, AutoLiveEntry>>,
  setMessages: Dispatch<SetStateAction<Message[]>>,
  sid: string,
): void {
  if (!autoLiveRef.current.has(sid)) return
  const entry = autoLiveRef.current.get(sid)!
  autoLiveRef.current.delete(sid)
  // Drop the ghost bubble whether it was synthesized (live-auto-<sid>) or seeded
  // from an inflight snapshot (id = the pre-allocated reply id).
  setMessages((prev) => prev.filter((m) => m.id !== entry.id && !m.id.startsWith(`live-auto-${sid}`)))
}

// recoverInflightSnapshot restores the in-progress assistant bubble after a
// MID-TURN reload. The turn keeps running detached server-side, but a fresh page
// only loads persisted messages — so the steps/agent produced before the reload
// vanish until the turn ends. This fetches the streaming snapshot (agent + text
// + steps-so-far, written to the crash sidecar on a throttle) and seeds it as the
// session's ghost bubble, which the session-step bus then keeps growing live. The
// authoritative message (same id) replaces it on completion. No-op when this
// window owns the run (its own SSE renders it) or the reply already persisted.
export async function recoverInflightSnapshot(
  ctx: AutoLiveContext,
  sid: string,
  loadedMsgs: Message[],
): Promise<void> {
  const { activeSessionIdRef, runsRef, autoLiveRef, setMessages, setPendingSessions } = ctx
  if (runsRef.current.has(sid)) return
  let snap: InflightSnapshot | null = null
  try {
    snap = await api.getInflight(sid)
  } catch {
    return
  }
  if (!snap || !snap.messageId) return
  if (loadedMsgs.some((m) => m.id === snap!.messageId)) return // already persisted
  if (activeSessionIdRef.current !== sid) return // user switched away mid-fetch
  let steps: TurnStep[] = []
  try {
    steps = JSON.parse(snap.steps || '[]') as TurnStep[]
  } catch {
    steps = []
  }

  // Race with the live bus: a `session_step` frame can arrive (foldAutoStep) and
  // create the ghost BEFORE this snapshot fetch resolves. When that happens the
  // ghost holds ONLY the steps emitted AFTER the reload (the bus never replays
  // history), while the snapshot holds the steps produced BEFORE it — with no
  // overlap. Merge by prepending the snapshot's older steps so the pre-reload
  // trace is restored ahead of the live ones, instead of being dropped (the
  // "old tool calls vanish, new ones keep showing" symptom). We keep the
  // existing ghost's id/text; only its step history grows at the front.
  const existing = autoLiveRef.current.get(sid)
  if (existing) {
    if (steps.length === 0) return
    existing.steps = [...steps, ...existing.steps]
    const json = JSON.stringify(existing.steps)
    const id = existing.id
    setMessages((prev) =>
      prev.map((m) =>
        m.id === id
          ? { ...m, steps: json, agentId: m.agentId || snap!.agentId || undefined }
          : m,
      ),
    )
    setPendingSessions((p) => withAdded(p, sid))
    return
  }

  // No live entry yet → seed a fresh ghost from the snapshot (the common case:
  // the snapshot fetch won the race, or no bus frame has arrived yet).
  autoLiveRef.current.set(sid, { id: snap.messageId, steps })
  const bubble: Message = {
    id: snap.messageId,
    sessionId: sid,
    role: 'assistant',
    agentId: snap.agentId || undefined,
    text: snap.text || '',
    steps: snap.steps || '[]',
    createdAt: Math.floor(Date.now() / 1000),
  }
  setMessages((prev) => (prev.some((m) => m.id === snap!.messageId) ? prev : [...prev, bubble]))
  setPendingSessions((p) => withAdded(p, sid))
}

// performReseedLive restores the live (unpersisted) assistant bubble THIS window
// is streaming after a session-switch reload wiped `messages` to the persisted-
// only list. It is the owning-window counterpart to recoverInflightSnapshot:
// recoverInflightSnapshot handles the NON-owning path (seeds a ghost from the
// server snapshot + bus), and early-returns when this window owns the run — so
// without reseedLive the owned bubble would stay gone until the next SSE frame
// re-created it. Uses the freshest bubble the local SSE handlers mirror into
// liveBubblesRef. No-op when no live bubble is owned, the user switched away, or
// it is already present.
export function performReseedLive(
  liveBubblesRef: RefObject<Map<string, Message>>,
  activeSessionIdRef: RefObject<string | null>,
  setMessages: Dispatch<SetStateAction<Message[]>>,
  sid: string,
  loadedMsgs: Message[],
): void {
  const bubble = liveBubblesRef.current.get(sid)
  if (!bubble) return
  if (activeSessionIdRef.current !== sid) return
  if (loadedMsgs.some((m) => m.id === bubble.id)) return
  setMessages((prev) => (prev.some((m) => m.id === bubble.id) ? prev : [...prev, bubble]))
}

// ---- inflight text polling (non-owning / post-refresh observer) ----

// The session-step bus deliberately DROPS delta/tool_delta frames (see
// sessionstep.go busForwardable) to avoid flooding the shared bus with token
// chunks — so a ghost bubble (recoverInflightSnapshot-seeded, bus-grown) shows
// the activity trace live but its ANSWER TEXT freezes at the snapshot taken when
// the page reloaded. This poll advances just that text: while the ACTIVE session
// has a running turn THIS window does not own, re-fetch the inflight snapshot
// every second and fold its (throttled, ≤600ms-fresh) text into the ghost
// bubble. Only `text` is touched — `steps` stays owned by the bus (applyAutoStep),
// so the two never fight. Stops the instant the turn ends (pending clears →
// transcript reload swaps in the authoritative message) or this window takes
// over the run. Returns the effect cleanup.
export function startInflightTextPoll(
  sid: string,
  activeSessionIdRef: RefObject<string | null>,
  runsRef: RefObject<Map<string, RunHandle>>,
  setMessages: Dispatch<SetStateAction<Message[]>>,
): () => void {
  let cancelled = false
  const tick = async () => {
    let snap: InflightSnapshot | null = null
    try {
      snap = await api.getInflight(sid)
    } catch {
      return
    }
    if (cancelled || !snap || !snap.messageId) return
    if (activeSessionIdRef.current !== sid) return
    if (runsRef.current.has(sid)) return // took over mid-poll
    const text = snap.text || ''
    setMessages((prev) => {
      const idx = prev.findIndex((m) => m.id === snap!.messageId)
      if (idx < 0 || prev[idx].text === text) return prev
      const next = prev.slice()
      next[idx] = { ...prev[idx], text }
      return next
    })
  }
  const iv = setInterval(() => void tick(), 1000)
  void tick()
  return () => {
    cancelled = true
    clearInterval(iv)
  }
}

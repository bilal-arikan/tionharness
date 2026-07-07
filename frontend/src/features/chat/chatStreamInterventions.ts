// In-flight turn interventions (stop / answer / interrupt / queue / steer) and
// the staged-item bookkeeping around them. Plain functions extracted from
// useChatStream; the hook's callbacks build the context and delegate here.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import { api } from '@/api'
import type { PendingAsk } from './AskPrompt'
import type { PendingItem } from './PendingTray'
import { STEER_GRACE_MS, pendingId, withRemoved, withoutKey } from './chatStreamHelpers'
import type { RunHandle, SendFn } from './chatStreamTypes'

// Context shared by the interventions that target the ACTIVE session's turn.
export interface TurnControlContext {
  activeSessionId: string | null
  runsRef: RefObject<Map<string, RunHandle>>
}

export interface StopContext extends TurnControlContext {
  setStreamingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setPendingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setPendingAsks: Dispatch<SetStateAction<Record<string, PendingAsk>>>
}

// Stop: cancel the active session's in-flight turn. The turn is detached from
// the SSE connection server-side, so aborting the fetch alone no longer stops
// generation — send an explicit "stop" control, then close the stream.
export function performStop(ctx: StopContext): void {
  const { activeSessionId, runsRef, setStreamingSessions, setPendingSessions, setPendingAsks } = ctx
  const sid = activeSessionId
  if (!sid) return
  const h = runsRef.current.get(sid)
  if (h?.runId) api.chatControl(h.runId, 'stop', '').catch(() => {})
  h?.ac.abort()
  setStreamingSessions((p) => withRemoved(p, sid))
  setPendingSessions((p) => withRemoved(p, sid))
  setPendingAsks((p) => withoutKey(p, sid))
}

export interface AnswerContext extends TurnControlContext {
  setPendingAsks: Dispatch<SetStateAction<Record<string, PendingAsk>>>
  setError: (msg: string | null) => void
}

// Answer: deliver the user's reply to the active session's turn paused on
// ask_user, resuming it.
export function performAnswerAsk(ctx: AnswerContext, text: string): void {
  const { activeSessionId, runsRef, setPendingAsks, setError } = ctx
  const sid = activeSessionId
  if (!sid) return
  setPendingAsks((p) => withoutKey(p, sid))
  const h = runsRef.current.get(sid)
  if (h?.runId) {
    api.chatControl(h.runId, 'answer', text).catch((e) => setError((e as Error).message))
  }
}

// Interrupt: stop the active session's turn and immediately send a new message
// to the SAME session.
export function performInterrupt(ctx: TurnControlContext, text: string, send: SendFn): void {
  const { activeSessionId, runsRef } = ctx
  const sid = activeSessionId
  if (!sid) return
  const h = runsRef.current.get(sid)
  if (h?.runId) api.chatControl(h.runId, 'stop', '').catch(() => {})
  h?.ac.abort()
  setTimeout(() => void send(text, sid), 0)
}

export interface QueueContext {
  activeSessionId: string | null
  setQueuedItems: Dispatch<SetStateAction<PendingItem[]>>
}

// Queue: stage a message to auto-send (FIFO) when the ACTIVE session's current
// turn finishes. Tagged with the session so it flushes to the right turn.
export function performQueueMessage(ctx: QueueContext, text: string): void {
  const { activeSessionId, setQueuedItems } = ctx
  const sid = activeSessionId
  if (!sid) return
  setQueuedItems((prev) => [...prev, { id: pendingId(), text, kind: 'queue', sid }])
}

export interface SteerContext extends TurnControlContext {
  steerTimers: RefObject<Map<string, ReturnType<typeof setTimeout>>>
  setQueuedItems: Dispatch<SetStateAction<PendingItem[]>>
  setError: (msg: string | null) => void
}

// Steer: stage live guidance with a short cancellable grace window, then POST
// it to the ACTIVE session's running turn.
export function performSteer(ctx: SteerContext, text: string): void {
  const { activeSessionId, runsRef, steerTimers, setQueuedItems, setError } = ctx
  const sid = activeSessionId
  if (!sid) return
  const id = pendingId()
  setQueuedItems((prev) => [...prev, { id, text, kind: 'steer', sid }])
  const timer = setTimeout(() => {
    steerTimers.current.delete(id)
    setQueuedItems((prev) => prev.filter((p) => p.id !== id))
    const h = runsRef.current.get(sid)
    if (h?.runId) {
      api.chatControl(h.runId, 'steer', text).catch((e) => setError((e as Error).message))
    }
  }, STEER_GRACE_MS)
  steerTimers.current.set(id, timer)
}

// Remove a staged item before it is processed.
export function performRemovePending(
  steerTimers: RefObject<Map<string, ReturnType<typeof setTimeout>>>,
  setQueuedItems: Dispatch<SetStateAction<PendingItem[]>>,
  id: string,
): void {
  const t = steerTimers.current.get(id)
  if (t) {
    clearTimeout(t)
    steerTimers.current.delete(id)
  }
  setQueuedItems((prev) => prev.filter((p) => p.id !== id))
}

// When a session's turn ends, drop any of ITS still-pending steers (their
// target run is gone) and cancel their grace timers. Used as the updater body
// of the hook's steer-cleanup effect.
export function dropStaleSteers(
  prev: PendingItem[],
  streamingSessions: ReadonlySet<string>,
  steerTimers: Map<string, ReturnType<typeof setTimeout>>,
): PendingItem[] {
  const stale = prev.filter((p) => p.kind === 'steer' && !streamingSessions.has(p.sid))
  if (stale.length === 0) return prev
  stale.forEach((p) => {
    const t = steerTimers.get(p.id)
    if (t) {
      clearTimeout(t)
      steerTimers.delete(p.id)
    }
  })
  const staleIds = new Set(stale.map((p) => p.id))
  return prev.filter((p) => !staleIds.has(p.id))
}

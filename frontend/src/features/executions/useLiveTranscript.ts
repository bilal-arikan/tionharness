// useLiveTranscript is the read-only, session-scoped live transcript used by
// non-chat views (ExecutionsPanel today; the flow-run viewer next). It gives
// those views the SAME live behaviour the chat has always had — folding
// session_step frames into a growing "ghost" assistant bubble and recovering an
// in-flight turn after a mid-turn reload — WITHOUT the chat's composer / owned-
// run / send machinery.
//
// It reuses the exact helpers behind the chat's ghost bubbles
// (foldAutoStep / recoverInflightSnapshot / clearAutoLiveEntry from
// chatStreamAutoLive) so the two screens converge on one live-transcript
// implementation. The live step frames arrive off the shared stepBus (fed by
// the single SSE feed in useAppEvents), so this opens no second EventSource.
//
// Data model per selected session:
//   - persisted messages   → api.listMessages (initial load + running poll +
//                            executions refresh tick + turn-end reload)
//   - live tool/thinking   → stepBus session_step frames → foldAutoStep ghost
//   - mid-turn reload seed  → api.getInflight via recoverInflightSnapshot
//   - turn end             → stepBus turn-end → clear ghost + reload authoritative
import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '@/api'
import type { Message } from '@/types'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_EXECUTIONS } from '@/app/eventToRefreshSignals'
import { subscribeStep, subscribeTurnEnd } from '@/shared/lib/stepBus'
import type { AutoLiveEntry, RunHandle } from '@/features/chat/chatStreamTypes'
import {
  foldAutoStep,
  recoverInflightSnapshot,
  clearAutoLiveEntry,
} from '@/features/chat/chatStreamAutoLive'

// Re-fetch cadence while the selected session's turn is still running, so newly
// persisted turns land without waiting for the next executions poll.
const RUNNING_POLL_MS = 2000

export interface LiveTranscript {
  messages: Message[]
  loading: boolean
}

// useLiveTranscript owns the transcript for `sessionId`. `running` drives the
// faster poll while the turn is live; onError surfaces a load failure.
export function useLiveTranscript(
  sessionId: string | null,
  running: boolean,
  onError?: (msg: string) => void,
): LiveTranscript {
  const [messages, setMessages] = useState<Message[]>([])
  const [loading, setLoading] = useState(false)
  // Mirror `running` into a ref so the (identity-stable) `load` callback can read
  // the latest turn state without being re-created on every running flip.
  const runningRef = useRef(running)
  runningRef.current = running

  // Ghost-bubble accumulator keyed by session (mirrors useChatStream). This view
  // never OWNS a run, so runsRef stays empty — that makes foldAutoStep /
  // recoverInflightSnapshot take their non-owning path unconditionally.
  const autoLiveRef = useRef<Map<string, AutoLiveEntry>>(new Map())
  const runsRef = useRef<Map<string, RunHandle>>(new Map())
  // Sessions whose inflight snapshot has already been merged this selection, so
  // the mid-turn seed runs EXACTLY ONCE regardless of whether a live bus step
  // (foldAutoStep) created the ghost first. Gating on autoLiveRef.has(sid)
  // instead would let that race skip the seed entirely — the snapshot fetch is
  // async, so a step arriving before it resolves would populate autoLiveRef and
  // block recovery, dropping every pre-reload tool call.
  const seededRef = useRef<Set<string>>(new Set())
  // The session this view is showing. foldAutoStep / recoverInflightSnapshot only
  // mutate the transcript when the frame's session matches this ref, so keep it
  // in sync with the selection synchronously each render.
  const activeSessionIdRef = useRef<string | null>(sessionId)
  activeSessionIdRef.current = sessionId
  // setPendingSessions is required by the shared helpers' context but this view
  // shows its own working indicator (via `loading` / `running`), so the pending
  // set itself is unused here.
  const [, setPendingSessions] = useState<ReadonlySet<string>>(() => new Set())

  const ctx = useRef({ activeSessionIdRef, runsRef, autoLiveRef, setMessages, setPendingSessions })

  // Load (or reload) the persisted transcript, then seed any in-flight snapshot.
  // `flash` controls the loading spinner: true on a fresh selection, false for
  // background reloads (poll / tick / turn-end) so the transcript doesn't blink.
  const load = useCallback(
    async (sid: string, flash: boolean) => {
      if (flash) setLoading(true)
      try {
        const m = await api.listMessages(sid)
        if (activeSessionIdRef.current !== sid) return
        // Reconcile the live ghost bubble with the freshly persisted list. A
        // background poll/tick previously replaced the whole array (dropping the
        // ghost) and then re-seeded it from the inflight snapshot on EVERY call —
        // which reset the bus-accumulated steps and churned the bubble id, so the
        // transcript flickered ("N araç" jumping, the bubble blinking to bare
        // working-dots) a few times a second while a turn ran.
        const ghostId = autoLiveRef.current.get(sid)?.id
        const ghostPersisted = !!ghostId && m.some((x) => x.id === ghostId)
        // Drop the accumulator once the turn's authoritative message has landed
        // (ghost id now in the list) or the turn is no longer running — otherwise
        // a stale entry would be appended to by the NEXT turn's steps.
        if (ghostId && (ghostPersisted || !runningRef.current)) {
          autoLiveRef.current.delete(sid)
        }
        setMessages((prev) => {
          // Keep the still-live ghost across the persisted-list swap so a poll
          // doesn't blink it out (it isn't persisted yet).
          if (runningRef.current && autoLiveRef.current.has(sid) && !ghostPersisted) {
            const ghost = prev.find((x) => x.id === ghostId)
            if (ghost) return [...m, ghost]
          }
          return m
        })
        // Seed/merge the inflight snapshot EXACTLY ONCE per selection — a genuine
        // mid-turn reload — not on every background poll. recoverInflightSnapshot
        // merges into a ghost a live bus step may have already created, so the
        // pre-reload trace is restored even when that step won the async race.
        if (!seededRef.current.has(sid)) {
          seededRef.current.add(sid)
          await recoverInflightSnapshot(ctx.current, sid, m)
        }
      } catch (e) {
        if (activeSessionIdRef.current === sid) onError?.((e as Error).message)
      } finally {
        if (flash && activeSessionIdRef.current === sid) setLoading(false)
      }
    },
    [onError],
  )

  // Selection change: reset + fresh load. Clear the previous session's ghost so a
  // stale bubble never bleeds across selections.
  useEffect(() => {
    if (!sessionId) {
      setMessages([])
      return
    }
    autoLiveRef.current.clear()
    seededRef.current.clear()
    void load(sessionId, true)
  }, [sessionId, load])

  // Cross-window live sync: the central dispatcher bumps the 'executions' signal
  // on every chat / flow / schedule / spawn / worker / task / session event.
  // Reload in the background (no spinner) so status + newly persisted turns stay
  // fresh, matching the list's own refresh.
  const tick = useRefreshTrigger(SIGNAL_EXECUTIONS)
  useEffect(() => {
    const sid = activeSessionIdRef.current
    if (sid) void load(sid, false)
  }, [tick, load])

  // Faster poll while the selected turn is running, so a completed turn's
  // persisted trace appears promptly even if its turn-end event is missed.
  useEffect(() => {
    if (!sessionId || !running) return
    const t = setInterval(() => void load(sessionId, false), RUNNING_POLL_MS)
    return () => clearInterval(t)
  }, [sessionId, running, load])

  // Live step frames off the shared bus → grow this view's ghost bubble in
  // lock-step with the chat, through the same fold helper.
  useEffect(() => {
    if (!sessionId) return
    return subscribeStep(sessionId, (step) => foldAutoStep(ctx.current, sessionId, step))
  }, [sessionId])

  // Turn end → drop the ghost + reload so the authoritative persisted turn (full
  // trace) replaces the live bubble.
  useEffect(() => {
    if (!sessionId) return
    return subscribeTurnEnd(sessionId, () => {
      clearAutoLiveEntry(autoLiveRef, setMessages, sessionId)
      void load(sessionId, false)
    })
  }, [sessionId, load])

  return { messages, loading }
}

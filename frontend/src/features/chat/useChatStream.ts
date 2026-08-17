// useChatStream owns the entire chat-turn streaming machinery: per-session
// streaming state, the SSE send loop, in-flight interventions (stop/answer/
// interrupt/queue/steer), the "/" slash commands and the derived view of the
// active session's turn. Extracted from App so the root component stays a
// composition + layout shell. The heavy per-concern logic lives in sibling
// modules (chatStreamSend/History/Interventions/AutoLive/Commands); this hook
// holds the React state and wires the callbacks to them.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { Attachment, Message, SlashCommand, TurnStep } from '@/types'
import type { PendingAsk } from './AskPrompt'
import type { PendingItem } from './PendingTray'
import { intersectWith, withAdded, withRemoved, withoutKey } from './chatStreamHelpers'
import type { AutoLiveEntry, ChatStreamDeps, WakeWait } from './chatStreamTypes'
import { performSend } from './chatStreamSend'
import { performRerunLast, performRetry, performRewindTo } from './chatStreamHistory'
import { clearAutoLiveEntry, performReseedLive } from './chatStreamAutoLive'
import { makeHubHandlers } from './chatStreamHub'
import { subscribeSessionStream, windowClientId } from '@/api/sessionStream'
import { buildChatCommands, performHandoff, performSummarize } from './chatStreamCommands'

export type { ChatStreamDeps } from './chatStreamTypes'

export function useChatStream(deps: ChatStreamDeps) {
  const {
    sessions,
    activeWorkspaceId,
    activeSessionId,
    activeAgentId,
    activeSessionIdRef,
    messagesRef,
    notifyEnabled,
    setMessages,
    setError,
    selectSession,
    refreshSessions,
    bumpMeter,
  } = deps

  // Streaming-turn control is PER-SESSION: several turns can overlap because a
  // turn is detached server-side and keeps running after the user switches
  // sessions. Tracking these per session (not as a single flag) keeps each
  // session's indicator, controls and staged interventions independent.
  const [streamingSessions, setStreamingSessions] = useState<ReadonlySet<string>>(() => new Set())
  const [pendingSessions, setPendingSessions] = useState<ReadonlySet<string>>(() => new Set())
  // Latest live (unpersisted) assistant bubble per session that THIS window owns.
  // Kept in a ref (not React state) so it survives a session-switch reload that
  // wipes `messages` to the persisted-only list: reseedLive re-injects it when the
  // user returns, and the local SSE handlers keep it in sync. Deleted the moment
  // the turn's reply persists (onReply) or fails (renderTurnError / finally).
  const liveBubblesRef = useRef<Map<string, Message>>(new Map())

  // Per-turn reasoning level picked in the composer ('' = use the agent's own
  // setting). Persisted so the choice carries across messages and reloads.
  const [thinkingLevel, setThinkingLevel] = useState(
    () => localStorage.getItem('tionswarm.thinkingLevel') ?? '',
  )
  const setThinkingLevelPersist = useCallback((v: string) => {
    setThinkingLevel(v)
    localStorage.setItem('tionswarm.thinkingLevel', v)
  }, [])

  // Per-turn permission-mode override picked in the composer ('' = use the
  // agent's own setting). Persisted across messages and reloads.
  const [permissionMode, setPermissionMode] = useState(
    () => localStorage.getItem('tionswarm.permissionMode') ?? '',
  )
  const setPermissionModePersist = useCallback((v: string) => {
    setPermissionMode(v)
    localStorage.setItem('tionswarm.permissionMode', v)
  }, [])

  // The ACTIVE session's WAITING backend queue (driven by queue_update hub
  // events) + its live viewer count (presence, Faz 4). Both are reset when the
  // subscription switches sessions.
  const [queued, setQueued] = useState<PendingItem[]>([])
  const [presence, setPresence] = useState(1)
  // "another window is typing…" — driven by the hub (typing events from OTHER
  // windows), auto-cleared if the peer stops updating (e.g. it closed).
  const [typingActive, setTypingActive] = useState(false)
  const typingClearTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const setTyping = useCallback((active: boolean) => {
    if (typingClearTimer.current) clearTimeout(typingClearTimer.current)
    setTypingActive(active)
    if (active) typingClearTimer.current = setTimeout(() => setTypingActive(false), 4000)
  }, [])
  // Outbound: the composer calls notifyTyping on each keystroke. First keystroke
  // pings active; an idle gap (2.5s) pings inactive. Throttled so it is not a
  // per-keystroke request storm.
  const typingSentRef = useRef(false)
  const typingIdleTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const notifyTyping = useCallback(() => {
    const sid = activeSessionId
    if (!sid) return
    if (!typingSentRef.current) {
      typingSentRef.current = true
      api.setTyping(sid, true, windowClientId).catch(() => {})
    }
    if (typingIdleTimer.current) clearTimeout(typingIdleTimer.current)
    typingIdleTimer.current = setTimeout(() => {
      typingSentRef.current = false
      api.setTyping(sid, false, windowClientId).catch(() => {})
    }, 2500)
  }, [activeSessionId])
  // ask_user pauses are held PER SESSION so a turn paused in one session does
  // not show its prompt while the user is viewing another.
  const [pendingAsks, setPendingAsks] = useState<Record<string, PendingAsk>>({})

  // Pending self-wakes: schedule_wake armed a turn to auto-resume after a delay.
  // Tracked PER SESSION (with the agent's reason + fire time) so the "waiting to
  // auto-resume" banner + its Durdur (cancel) control show on the right session.
  // The turn that armed it has ENDED, so without this the session looks finished.
  const [wakeWaits, setWakeWaits] = useState<Record<string, WakeWait>>({})

  const sendMessage = useCallback(
    // `targetSid` lets a queued-message flush (or interrupt) send to a specific
    // session even if the user has since switched away; defaults to the active
    // session for normal sends. The turn itself runs in performSend.
    async (text: string, targetSid?: string, attachments: Attachment[] = []) => {
      await performSend(
        {
          activeSessionId,
          sessions,
          thinkingLevel,
          permissionMode,
          setPendingSessions,
          setQueued,
          setWakeWaits,
          setError,
        },
        text,
        targetSid,
        attachments,
      )
    },
    [activeSessionId, sessions, thinkingLevel, permissionMode, setError],
  )

  // Keep a live ref to sendMessage so effects (queue flush) can call the latest
  // version without listing it as a dependency or hitting TDZ.
  const sendMessageRef = useRef(sendMessage)
  useEffect(() => {
    sendMessageRef.current = sendMessage
  }, [sendMessage])

  // retryMessage re-runs the turn behind a failed assistant bubble (see
  // performRetry for the mechanics). The destructive variant used by interactive
  // chat: it deletes the failed pair before re-sending so history stays clean.
  const retryMessage = useCallback(
    async (failedId: string) => {
      await performRetry({ activeSessionIdRef, messagesRef, setMessages, sendMessageRef }, failedId)
    },
    [activeSessionIdRef, messagesRef, setMessages],
  )

  // retryMessagePreserve is the read-only run-log variant: it re-runs the failed
  // turn WITHOUT deleting the transcript (a task / flow / schedule log is an
  // audit trail). Wired by ChatView for read-only sessions so a hit-limit turn
  // there can still be retried from its error card without rewriting history.
  const retryMessagePreserve = useCallback(
    async (failedId: string) => {
      await performRetry(
        { activeSessionIdRef, messagesRef, setMessages, sendMessageRef },
        failedId,
        true,
      )
    },
    [activeSessionIdRef, messagesRef, setMessages],
  )

  // rerunLast re-runs the most recent turn of the active session — the "restart"
  // action in the Session Info panel (see performRerunLast).
  const rerunLast = useCallback(async () => {
    await performRerunLast({ activeSessionIdRef, messagesRef, sendMessageRef }, retryMessage)
  }, [activeSessionIdRef, messagesRef, retryMessage])

  // Rewind ("/rewind"): conversation-only checkpoint restore. The command opens a
  // picker (rewindOpen) listing the session's user prompts; choosing one truncates
  // the transcript back to that checkpoint. File changes are NOT reverted.
  const [rewindOpen, setRewindOpen] = useState(false)
  const openRewind = useCallback(() => setRewindOpen(true), [])
  const closeRewind = useCallback(() => setRewindOpen(false), [])

  // rewindTo removes the anchor message and everything after it (see
  // performRewindTo); returns the removed prompt text for the composer.
  const rewindTo = useCallback(
    async (messageId: string): Promise<string> =>
      performRewindTo({ activeSessionIdRef, messagesRef, setMessages }, setRewindOpen, messageId),
    [activeSessionIdRef, messagesRef, setMessages],
  )

  // The backend serial queue replaces the old client-side flush loop: sending
  // while a turn runs just enqueues (POST /messages) and the server dispatches in
  // order. Waiting items arrive via queue_update; there is nothing to flush here.

  // ---- turn interventions (session-scoped: the queue runs turns server-side) ----

  // Stop: cancel the active session's in-flight turn. Session-scoped — the client
  // no longer holds a runId (the worker owns the run), so the server resolves it.
  const stopTurn = useCallback(() => {
    const sid = activeSessionId
    if (!sid) return
    setStreamingSessions((p) => withRemoved(p, sid))
    setPendingSessions((p) => withRemoved(p, sid))
    setPendingAsks((p) => withoutKey(p, sid))
    api.sessionControl(sid, 'stop').catch(() => {})
  }, [activeSessionId])

  // Answer: resolve the active session's pending interaction (ask_user /
  // permission / plan) via the resolve-once endpoint. Optimistically close the
  // card here; the server broadcasts interaction_resolved so every OTHER window
  // closes it too. A 409 (another window answered first) is expected and benign.
  const answerAsk = useCallback(
    (text: string) => {
      const sid = activeSessionId
      if (!sid) return
      const ask = pendingAsks[sid]
      setPendingAsks((p) => withoutKey(p, sid))
      if (ask?.interactionId) {
        api.answerInteraction(sid, ask.interactionId, { answer: text }).catch(() => {})
      }
    },
    [activeSessionId, pendingAsks],
  )

  // Interrupt: stop the active session's turn, then enqueue a new message to the
  // SAME session (it dispatches once the stopped turn unwinds).
  const interruptTurn = useCallback(
    (text: string, attachments?: Attachment[]) => {
      const sid = activeSessionId
      if (!sid) return
      api.sessionControl(sid, 'stop').catch(() => {})
      void sendMessage(text, sid, attachments)
    },
    [activeSessionId, sendMessage],
  )

  // ---- post-reload recovery (detached turns still running server-side) ----

  // Seed the "thinking" indicator for sessions whose turn is still in flight
  // after a page reload (queried via api.activeSessions). Only the pending set
  // is touched — not streaming — so the composer keeps its Send button (there is
  // no local run handle to stop/steer after a reload).
  const markPending = useCallback((ids: string[]) => {
    if (ids.length === 0) return
    setPendingSessions((p) => {
      let next = p
      for (const id of ids) next = withAdded(next, id)
      return next
    })
  }, [])

  // reconcileActive REPLACES the local running latches with the server's list of
  // in-flight sessions for the workspace now on screen. Called on every workspace
  // switch (and the initial resolve), where the server is authoritative and this
  // window owns no live run in the workspace it just entered.
  //
  // markPending only ever ADDS, so nothing could drop a stale latch. That leaked:
  // the global completion feed clears a session's latch only while ITS workspace
  // is the active one (useAppEvents gates on e.workspaceId), so a turn that ends
  // after the user switched away stays latched for the lifetime of the tab. And
  // session ids are per-workspace counters (each store issues its own SES1, SES2,
  // …), so that orphaned id collides with a real session in the workspace now on
  // screen — lighting its nav "Sohbet" busy dot, sidebar spinner and composer
  // Durdur button with no run behind them.
  const reconcileActive = useCallback((ids: string[]) => {
    const live = new Set(ids)
    setPendingSessions(() => live)
    setStreamingSessions((p) => intersectWith(p, live))
  }, [])

  // Clear a session's post-reload pending indicator once its turn has ended
  // (chat-completion event). Locally-owned live streams clear themselves.
  // Called from the GLOBAL completion feed (useAppEvents: chat/spawned/worker/…),
  // which arrives regardless of which session is on screen. Clears BOTH pending
  // and streaming — the latter matters when a turn finished for a session the
  // user had navigated away from: its per-session hub subscription was torn down,
  // so its turn_done never cleared streamingSessions, leaving a stuck "Durdur"
  // (stale running indicator) on return. This global signal is the authoritative
  // turn-end that covers the off-screen case.
  const clearPending = useCallback((sid: string) => {
    setPendingSessions((p) => withRemoved(p, sid))
    setStreamingSessions((p) => withRemoved(p, sid))
  }, [])

  // ---- autonomous / other-window live turns (session-step bus) ----

  // Accumulators behind the per-session "ghost" assistant bubbles (see
  // chatStreamAutoLive for the full story). Keyed by session because turns are
  // detached and several can overlap.
  const autoLiveRef = useRef<Map<string, AutoLiveEntry>>(new Map())

  // applyAutoStep: rendering is now the hub subscription's job (authoritative,
  // per ACTIVE session). A global bus step frame only raises the "pending"
  // indicator for a session that is NOT on screen here; when the user opens it,
  // the hub stream replays its history and renders it. No more ghost bubble from
  // the bus (that path is the hub's now).
  const applyAutoStep = useCallback(
    (sid: string, _step: TurnStep) => {
      if (activeSessionIdRef.current === sid) return
      setPendingSessions((p) => withAdded(p, sid))
    },
    [activeSessionIdRef],
  )

  // clearAutoLive drops a session's ghost bubble + its accumulator once the turn
  // has ended (see clearAutoLiveEntry).
  const clearAutoLive = useCallback(
    (sid: string) => {
      clearAutoLiveEntry(autoLiveRef, setMessages, sid)
    },
    [setMessages],
  )

  // Mid-turn reload recovery is the hub subscription's job now: on (re)connect it
  // replays the in-flight turn from the server ring, rebuilding the live bubble —
  // no separate inflight-snapshot fetch. Kept as a no-op so the controller's
  // message-load call site is unchanged.
  const recoverInflight = useCallback(async (_sid: string, _loadedMsgs: Message[]) => {}, [])

  // reseedLive restores the live (unpersisted) assistant bubble THIS window is
  // streaming after a session-switch reload (see performReseedLive).
  const reseedLive = useCallback(
    (sid: string, loadedMsgs: Message[]) => {
      performReseedLive(liveBubblesRef, activeSessionIdRef, setMessages, sid, loadedMsgs)
    },
    [activeSessionIdRef, setMessages],
  )

  // ---- inflight text polling (non-owning / post-refresh observer) ----

  // Authoritative render: subscribe the ACTIVE session to its per-session hub
  // stream and render the transcript live from it — full cutover, so EVERY window
  // (the sender and every other viewer) renders from here, not from the send
  // request's own SSE. Cursor-based: a reconnect gap-fills from the server ring,
  // and a reset (server restart / eviction) triggers a full listMessages resync.
  // Replaces the old bus-ghost + inflight-text-polling machinery.
  useEffect(() => {
    const sid = activeSessionId
    // No workspace → no identifiable session (ids repeat across stores), so there
    // is nothing to subscribe to yet.
    if (!sid || !activeWorkspaceId) return
    // Fresh session view: clear the previous session's queue/presence until this
    // one's first queue_update / presence frame arrives.
    setQueued([])
    setPresence(1)
    setTypingActive(false)
    const reload = () => {
      api
        .listMessages(sid)
        .then(setMessages)
        .catch(() => {})
    }
    const handlers = makeHubHandlers({
      sid,
      activeSessionIdRef,
      setMessages,
      setStreamingSessions,
      setPendingSessions,
      setPendingAsks,
      setQueued,
      setPresence,
      setTyping,
      reload,
      notifyEnabled,
      bumpMeter,
    })
    return subscribeSessionStream(sid, handlers)
  }, [
    activeWorkspaceId,
    activeSessionId,
    activeSessionIdRef,
    setMessages,
    setTyping,
    notifyEnabled,
    bumpMeter,
  ])

  // ---- self-wake (schedule_wake) waiting state ----

  // Arm the waiting banner: a schedule_wake event (phase=armed) said this session
  // will auto-resume after a delay. reason/fireAt come from the event target.
  const setWakeWait = useCallback((sid: string, reason: string, fireAt: number) => {
    setWakeWaits((p) => ({ ...p, [sid]: { reason, fireAt } }))
  }, [])

  // Clear the waiting banner (wake fired, was cancelled, or the turn ended).
  const clearWakeWait = useCallback((sid: string) => {
    setWakeWaits((p) => withoutKey(p, sid))
  }, [])

  // Durdur: disarm the active session's pending self-wake. Optimistically clear
  // the banner, then POST; the server also emits phase=cancelled which clears it.
  const cancelWake = useCallback(() => {
    const sid = activeSessionId
    if (!sid) return
    setWakeWaits((p) => withoutKey(p, sid))
    api.cancelWake(sid).catch((e) => setError((e as Error).message))
  }, [activeSessionId, setError])

  // Queue: with the backend serial queue, "queue" is just a normal send — the
  // server serialises it behind the running turn and shows it in the tray.
  const queueMessage = useCallback(
    (text: string, attachments?: Attachment[]) => {
      void sendMessage(text, undefined, attachments)
    },
    [sendMessage],
  )

  // Steer: inject live guidance into the ACTIVE session's running turn
  // (session-scoped; the worker owns the run). Native providers fold it in via the
  // steer channel; claude-cli stashes it and delivers it at the next tool boundary
  // (server reports "steered"). If that turn ends with no tool call, the backend
  // itself enqueues the message as the next turn (steer_undelivered fallback), so
  // the client needs no fallback here anymore. The server reports "unsupported" when
  // a steer cannot reach the turn — a claude-cli agent in "auto" mode (no permission-
  // prompt boundary), or an older backend — in which case we queue the message and
  // tell the user, instead of silently dropping their guidance.
  const steerTurn = useCallback(
    (text: string) => {
      const sid = activeSessionId
      if (!sid || !text.trim()) return
      api
        .sessionControl(sid, 'steer', text)
        .then((r) => {
          if (r?.result === 'unsupported') {
            void sendMessage(text)
            setError(
              'Auto izin modunda canlı yönlendirme desteklenmiyor (claude-cli) — mesaj sıraya alındı. Canlı yönlendirme için ajanı "ask" moduna al.',
            )
          }
        })
        .catch((e) => setError((e as Error).message))
    },
    [activeSessionId, setError, sendMessage],
  )

  // Remove a WAITING queued message before it is dispatched (backend cancel).
  const removePending = useCallback(
    (id: string) => {
      const sid = activeSessionId
      if (!sid) return
      setQueued((prev) => prev.filter((p) => p.id !== id))
      api.cancelQueued(sid, id).catch(() => {})
    },
    [activeSessionId],
  )

  // Clear the whole waiting queue for the active session.
  const clearQueue = useCallback(() => {
    const sid = activeSessionId
    if (!sid) return
    setQueued([])
    api.clearQueue(sid).catch(() => {})
  }, [activeSessionId])

  // Promote a waiting message so it dispatches next ("öne al").
  const sendQueuedNext = useCallback(
    (id: string) => {
      const sid = activeSessionId
      if (!sid) return
      setQueued((prev) => {
        const it = prev.find((p) => p.id === id)
        return it ? [it, ...prev.filter((p) => p.id !== id)] : prev
      })
      api.moveQueuedFront(sid, id).catch(() => {})
    },
    [activeSessionId],
  )

  // Run a "/" command that posts an assistant message: summary (board/flows/
  // tools) or conversation compaction (see performSummarize).
  const summarize = useCallback(
    (kind: string) => {
      performSummarize({ activeSessionId, activeAgentId, setMessages, setError }, kind)
    },
    [activeSessionId, activeAgentId, setMessages, setError],
  )

  // Context reset (/handoff): write a handoff artifact and continue in a fresh
  // session (see performHandoff).
  const handoff = useCallback(() => {
    performHandoff({
      activeSessionId,
      activeAgentId,
      setMessages,
      setError,
      refreshSessions,
      selectSession,
    })
  }, [activeSessionId, activeAgentId, setMessages, setError, refreshSessions, selectSession])

  // Slash commands available in the chat composer ("/" menu): built-in session
  // commands only (running a flow from the composer was removed — flows run from
  // the Flows panel; see buildChatCommands).
  const chatCommands = useMemo<SlashCommand[]>(
    () => buildChatCommands({ summarize, handoff, openRewind }),
    [summarize, handoff, openRewind],
  )

  // Derive the active session's view of the per-session streaming state.
  const activeStreaming = activeSessionId ? streamingSessions.has(activeSessionId) : false
  const activePending = activeSessionId ? pendingSessions.has(activeSessionId) : false
  const activeAsk = activeSessionId ? (pendingAsks[activeSessionId] ?? null) : null
  const activeWakeWait = activeSessionId ? (wakeWaits[activeSessionId] ?? null) : null
  // The active session's WAITING backend queue is already session-scoped (the hub
  // subscription is per active session), so it maps straight through.
  const activeQueued = queued
  // "open in N windows" — >1 means another window is also viewing this session.
  const activePresence = presence

  return {
    streamingSessions,
    activePresence,
    activeTyping: typingActive,
    notifyTyping,
    thinkingLevel,
    setThinkingLevel: setThinkingLevelPersist,
    permissionMode,
    setPermissionMode: setPermissionModePersist,
    sendMessage,
    retryMessage,
    retryMessagePreserve,
    rerunLast,
    stopTurn,
    answerAsk,
    interruptTurn,
    queueMessage,
    steerTurn,
    removePending,
    clearQueue,
    sendQueuedNext,
    markPending,
    reconcileActive,
    clearPending,
    applyAutoStep,
    clearAutoLive,
    recoverInflight,
    reseedLive,
    setWakeWait,
    clearWakeWait,
    cancelWake,
    summarize,
    chatCommands,
    rewindOpen,
    openRewind,
    closeRewind,
    rewindTo,
    activeStreaming,
    activePending,
    activeAsk,
    activeWakeWait,
    activeQueued,
  }
}

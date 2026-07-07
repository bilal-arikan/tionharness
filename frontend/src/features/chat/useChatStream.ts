// useChatStream owns the entire chat-turn streaming machinery: per-session
// streaming state, the SSE send loop, in-flight interventions (stop/answer/
// interrupt/queue/steer), the "/" slash commands and the derived view of the
// active session's turn. Extracted from App so the root component stays a
// composition + layout shell. The heavy per-concern logic lives in sibling
// modules (chatStreamSend/History/Interventions/AutoLive/Commands); this hook
// holds the React state and wires the callbacks to them.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { Attachment, Flow, Message, SlashCommand, TurnStep } from '@/types'
import type { PendingAsk } from './AskPrompt'
import type { PendingItem } from './PendingTray'
import { withAdded, withRemoved, withoutKey } from './chatStreamHelpers'
import type { AutoLiveEntry, ChatStreamDeps, RunHandle, WakeWait } from './chatStreamTypes'
import { performSend } from './chatStreamSend'
import { performRerunLast, performRetry, performRewindTo } from './chatStreamHistory'
import {
  dropStaleSteers,
  performAnswerAsk,
  performInterrupt,
  performQueueMessage,
  performRemovePending,
  performSteer,
  performStop,
} from './chatStreamInterventions'
import {
  clearAutoLiveEntry,
  foldAutoStep,
  performReseedLive,
  recoverInflightSnapshot,
  startInflightTextPoll,
} from './chatStreamAutoLive'
import {
  buildChatCommands,
  performHandoff,
  performRunFlow,
  performSummarize,
} from './chatStreamCommands'

export type { ChatStreamDeps } from './chatStreamTypes'

export function useChatStream(deps: ChatStreamDeps) {
  const {
    sessions,
    activeSessionId,
    activeAgentId,
    activeSessionIdRef,
    messagesRef,
    notifyEnabled,
    setMessages,
    setError,
    setView,
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
  const runsRef = useRef<Map<string, RunHandle>>(new Map())
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

  // Staged interventions shown above the composer while a turn streams.
  const [queuedItems, setQueuedItems] = useState<PendingItem[]>([])
  const steerTimers = useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map())
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
          activeSessionIdRef,
          notifyEnabled,
          runsRef,
          liveBubblesRef,
          setMessages,
          setStreamingSessions,
          setPendingSessions,
          setPendingAsks,
          setWakeWaits,
          setError,
          setView,
          selectSession,
          refreshSessions,
          bumpMeter,
        },
        text,
        targetSid,
        attachments,
      )
    },
    [activeSessionId, sessions, thinkingLevel, permissionMode, activeSessionIdRef, notifyEnabled, setMessages, setError, setView, selectSession, refreshSessions, bumpMeter],
  )

  // Keep a live ref to sendMessage so effects (queue flush) can call the latest
  // version without listing it as a dependency or hitting TDZ.
  const sendMessageRef = useRef(sendMessage)
  useEffect(() => {
    sendMessageRef.current = sendMessage
  }, [sendMessage])

  // retryMessage re-runs the turn behind a failed assistant bubble (see
  // performRetry for the mechanics).
  const retryMessage = useCallback(
    async (failedId: string) => {
      await performRetry({ activeSessionIdRef, messagesRef, setMessages, sendMessageRef }, failedId)
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

  // When a session's turn ends, drop any of ITS still-pending steers (their
  // target run is gone) and cancel their grace timers.
  useEffect(() => {
    setQueuedItems((prev) => dropStaleSteers(prev, streamingSessions, steerTimers.current))
  }, [streamingSessions])

  // When a session's turn ends, flush the next message queued FOR THAT SESSION
  // (FIFO). Sending re-adds the session to streamingSessions, so this re-runs
  // for the following queued item in order.
  useEffect(() => {
    const next = queuedItems.find((p) => p.kind === 'queue' && !streamingSessions.has(p.sid))
    if (next) {
      setQueuedItems((prev) => prev.filter((p) => p.id !== next.id))
      void sendMessageRef.current(next.text, next.sid)
    }
  }, [streamingSessions, queuedItems])

  // ---- streaming-turn interventions (target the ACTIVE session's turn) ----

  // Stop: cancel the active session's in-flight turn (see performStop).
  const stopTurn = useCallback(() => {
    performStop({ activeSessionId, runsRef, setStreamingSessions, setPendingSessions, setPendingAsks })
  }, [activeSessionId])

  // Answer: deliver the user's reply to the active session's turn paused on
  // ask_user, resuming it.
  const answerAsk = useCallback((text: string) => {
    performAnswerAsk({ activeSessionId, runsRef, setPendingAsks, setError }, text)
  }, [activeSessionId, setError])

  // Interrupt: stop the active session's turn and immediately send a new message
  // to the SAME session.
  const interruptTurn = useCallback(
    (text: string) => {
      performInterrupt({ activeSessionId, runsRef }, text, sendMessage)
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

  // Clear a session's post-reload pending indicator once its turn has ended
  // (chat-completion event). Locally-owned live streams clear themselves.
  const clearPending = useCallback((sid: string) => {
    setPendingSessions((p) => withRemoved(p, sid))
  }, [])

  // ---- autonomous / other-window live turns (session-step bus) ----

  // Accumulators behind the per-session "ghost" assistant bubbles (see
  // chatStreamAutoLive for the full story). Keyed by session because turns are
  // detached and several can overlap.
  const autoLiveRef = useRef<Map<string, AutoLiveEntry>>(new Map())

  // applyAutoStep folds one bus step into the active session's ghost bubble
  // (see foldAutoStep).
  const applyAutoStep = useCallback(
    (sid: string, step: TurnStep) => {
      foldAutoStep({ activeSessionIdRef, runsRef, autoLiveRef, setMessages, setPendingSessions }, sid, step)
    },
    [activeSessionIdRef, setMessages],
  )

  // clearAutoLive drops a session's ghost bubble + its accumulator once the turn
  // has ended (see clearAutoLiveEntry).
  const clearAutoLive = useCallback(
    (sid: string) => {
      clearAutoLiveEntry(autoLiveRef, setMessages, sid)
    },
    [setMessages],
  )

  // recoverInflight restores the in-progress assistant bubble after a MID-TURN
  // reload (see recoverInflightSnapshot).
  const recoverInflight = useCallback(
    async (sid: string, loadedMsgs: Message[]) => {
      await recoverInflightSnapshot(
        { activeSessionIdRef, runsRef, autoLiveRef, setMessages, setPendingSessions },
        sid,
        loadedMsgs,
      )
    },
    [activeSessionIdRef, setMessages],
  )

  // reseedLive restores the live (unpersisted) assistant bubble THIS window is
  // streaming after a session-switch reload (see performReseedLive).
  const reseedLive = useCallback(
    (sid: string, loadedMsgs: Message[]) => {
      performReseedLive(liveBubblesRef, activeSessionIdRef, setMessages, sid, loadedMsgs)
    },
    [activeSessionIdRef, setMessages],
  )

  // ---- inflight text polling (non-owning / post-refresh observer) ----

  // While the ACTIVE session has a running turn THIS window does not own, poll
  // the inflight snapshot to advance the ghost bubble's answer text (the bus
  // drops delta frames — see startInflightTextPoll for the full story).
  useEffect(() => {
    const sid = activeSessionId
    if (!sid) return
    if (!pendingSessions.has(sid)) return
    if (runsRef.current.has(sid)) return // this window owns the turn — its own SSE streams the text
    return startInflightTextPoll(sid, activeSessionIdRef, runsRef, setMessages)
  }, [activeSessionId, pendingSessions, activeSessionIdRef, setMessages])

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

  // Queue: stage a message to auto-send (FIFO) when the ACTIVE session's current
  // turn finishes (see performQueueMessage).
  const queueMessage = useCallback((text: string) => {
    performQueueMessage({ activeSessionId, setQueuedItems }, text)
  }, [activeSessionId])

  // Steer: stage live guidance with a short cancellable grace window, then POST
  // it to the ACTIVE session's running turn (see performSteer).
  const steerTurn = useCallback((text: string) => {
    performSteer({ activeSessionId, runsRef, steerTimers, setQueuedItems, setError }, text)
  }, [activeSessionId, setError])

  // Remove a staged item before it is processed.
  const removePending = useCallback((id: string) => {
    performRemovePending(steerTimers, setQueuedItems, id)
  }, [])

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
    performHandoff({ activeSessionId, activeAgentId, setMessages, setError, refreshSessions, selectSession })
  }, [activeSessionId, activeAgentId, setMessages, setError, refreshSessions, selectSession])

  // Run a flow from the chat composer, streaming node-by-node progress over SSE
  // (see performRunFlow).
  const runFlow = useCallback(
    (flowId: string, flowName: string, input: string, attachments: Attachment[] = []) => {
      performRunFlow({ activeSessionId, activeAgentId, setMessages, setError }, flowId, flowName, input, attachments)
    },
    [activeSessionId, activeAgentId, setMessages, setError],
  )

  // Flow list for the "/" command palette (each flow becomes a slash command).
  // Fetched once on mount; flows change rarely and the menu reads the latest list.
  const [flows, setFlows] = useState<Flow[]>([])
  useEffect(() => {
    api.listFlows().then(setFlows).catch(() => {})
  }, [])

  // Slash commands available in the chat composer ("/" menu): built-in session
  // commands plus one entry per flow (see buildChatCommands).
  const chatCommands = useMemo<SlashCommand[]>(
    () => buildChatCommands({ flows, summarize, handoff, openRewind, runFlow }),
    [summarize, handoff, openRewind, flows, runFlow],
  )

  // Derive the active session's view of the per-session streaming state.
  const activeStreaming = activeSessionId ? streamingSessions.has(activeSessionId) : false
  const activePending = activeSessionId ? pendingSessions.has(activeSessionId) : false
  const activeAsk = activeSessionId ? pendingAsks[activeSessionId] ?? null : null
  const activeWakeWait = activeSessionId ? wakeWaits[activeSessionId] ?? null : null
  const activeQueued = useMemo(
    () => (activeSessionId ? queuedItems.filter((p) => p.sid === activeSessionId) : []),
    [queuedItems, activeSessionId],
  )

  return {
    streamingSessions,
    thinkingLevel,
    setThinkingLevel: setThinkingLevelPersist,
    permissionMode,
    setPermissionMode: setPermissionModePersist,
    sendMessage,
    retryMessage,
    rerunLast,
    stopTurn,
    answerAsk,
    interruptTurn,
    queueMessage,
    steerTurn,
    removePending,
    markPending,
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

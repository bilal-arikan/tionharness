// useChatStream owns the entire chat-turn streaming machinery: per-session
// streaming state, the SSE send loop, in-flight interventions (stop/answer/
// interrupt/queue/steer), the "/" slash commands and the derived view of the
// active session's turn. Extracted from App so the root component stays a
// composition + layout shell.
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Dispatch,
  type RefObject,
  type SetStateAction,
} from 'react'
import { api } from '../api'
import type { Agent, Attachment, Flow, Message, Session, SlashCommand, TurnStep } from '../types'
import type { View } from '../components/NavRail'
import type { PendingAsk } from '../components/chat/AskPrompt'
import type { PendingItem } from '../components/chat/PendingTray'
import { notify } from '../lib/clientPrefs'

// Grace window before a staged steer is actually POSTed to the running turn
// (removing the item within this window cancels it).
const STEER_GRACE_MS = 3000

// Immutable Set/Record helpers for the per-session streaming state.
function withAdded(prev: ReadonlySet<string>, v: string): ReadonlySet<string> {
  if (prev.has(v)) return prev
  const n = new Set(prev)
  n.add(v)
  return n
}
function withRemoved(prev: ReadonlySet<string>, v: string): ReadonlySet<string> {
  if (!prev.has(v)) return prev
  const n = new Set(prev)
  n.delete(v)
  return n
}
function withoutKey<T>(prev: Record<string, T>, key: string): Record<string, T> {
  if (!(key in prev)) return prev
  const n = { ...prev }
  delete n[key]
  return n
}

// flowSlug turns a flow name into a space-less "/" command token (Turkish chars
// folded to ASCII), e.g. "Geri Bildirim Yönlendirici" → "geri-bildirim-yonlendirici".
function flowSlug(name: string): string {
  const map: Record<string, string> = { ı: 'i', İ: 'i', ş: 's', ğ: 'g', ü: 'u', ö: 'o', ç: 'c' }
  return name
    .replace(/[ıİşğüöç]/g, (c) => map[c] ?? c)
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export interface ChatStreamDeps {
  agents: Agent[]
  sessions: Session[]
  activeSessionId: string | null
  activeAgentId: string | null
  // Ref to the active session id so the detached SSE callbacks can tell whether
  // an update belongs to the session currently on screen.
  activeSessionIdRef: RefObject<string | null>
  // Live view of the open transcript, so retry can find the user prompt behind a
  // failed turn without re-binding callbacks on every message change.
  messagesRef: RefObject<Message[]>
  // Live desktop-notification preference (read without re-binding callbacks).
  notifyEnabled: RefObject<boolean>
  setMessages: Dispatch<SetStateAction<Message[]>>
  setError: (msg: string | null) => void
  setView: (v: View) => void
  selectSession: (id: string) => void
  refreshSessions: () => void
  // Bump the context/budget meter refresh counter after a turn completes.
  bumpMeter: () => void
}

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
  const runsRef = useRef<Map<string, { runId: string; ac: AbortController }>>(new Map())

  // Per-turn reasoning level picked in the composer ('' = use the agent's own
  // setting). Persisted so the choice carries across messages and reloads.
  const [thinkingLevel, setThinkingLevel] = useState(
    () => localStorage.getItem('swarmgo.thinkingLevel') ?? '',
  )
  const setThinkingLevelPersist = useCallback((v: string) => {
    setThinkingLevel(v)
    localStorage.setItem('swarmgo.thinkingLevel', v)
  }, [])

  // Per-turn permission-mode override picked in the composer ('' = use the
  // agent's own setting). Persisted across messages and reloads.
  const [permissionMode, setPermissionMode] = useState(
    () => localStorage.getItem('swarmgo.permissionMode') ?? '',
  )
  const setPermissionModePersist = useCallback((v: string) => {
    setPermissionMode(v)
    localStorage.setItem('swarmgo.permissionMode', v)
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
  const [wakeWaits, setWakeWaits] = useState<Record<string, { reason: string; fireAt: number }>>({})

  const sendMessage = useCallback(
    // `targetSid` lets a queued-message flush (or interrupt) send to a specific
    // session even if the user has since switched away; defaults to the active
    // session for normal sends.
    async (text: string, targetSid?: string, attachments: Attachment[] = []) => {
      const sid = targetSid ?? activeSessionId
      if (!sid) return
      setError(null)

      // The message is answered by the session's bound agent (chosen from the
      // composer dropdown). "@mention" routing was removed — one agent per turn.
      const sessAgent = sessions.find((s) => s.id === sid)?.agentId
      const agentIds = sessAgent ? [sessAgent] : []

      const now = Math.floor(Date.now() / 1000)
      const optimistic: Message = {
        id: `tmp-${Date.now()}`,
        sessionId: sid,
        role: 'user',
        text,
        attachments: attachments.length ? attachments : undefined,
        createdAt: now,
      }
      // Apply a message update only while THIS turn's session is the one on
      // screen. The turn is detached server-side, so its SSE callbacks keep
      // firing after the user switches sessions; without this guard they would
      // write the live bubble / deltas into whichever session is now displayed,
      // mixing the two transcripts. The reply still persists server-side and
      // reloads when the user returns to `sid`.
      const onSid = (updater: (prev: Message[]) => Message[]) =>
        setMessages((prev) => (activeSessionIdRef.current === sid ? updater(prev) : prev))

      onSid((prev) => [...prev, optimistic])
      // The user is taking over with a fresh message; any pending self-wake for
      // this session is now moot — drop its waiting banner.
      setWakeWaits((p) => withoutKey(p, sid))
      setPendingSessions((p) => withAdded(p, sid))
      const ac = new AbortController()
      runsRef.current.set(sid, { runId: '', ac })
      setStreamingSessions((p) => withAdded(p, sid))

      // The live bubble for the agent currently answering (multi-agent turns
      // produce several bubbles, one per agent, in order).
      let liveId = ''
      let liveSteps: TurnStep[] = []

      // Render a turn failure INTO the transcript (not just a top banner) so the
      // error is visible in the message hierarchy with its detail and survives a
      // reload when the server persisted it. The user message is kept; the live
      // (unsaved) bubble is replaced by the error bubble. When the server sent a
      // persisted error message (replyMessage) we show that; otherwise — a
      // client-side/transport failure — we synthesize a local error bubble.
      const renderTurnError = (detail: string, replyMessage?: Message) => {
        setPendingAsks((p) => withoutKey(p, sid))
        onSid((prev) => {
          const base = prev.filter((m) => !m.id.startsWith('live-'))
          if (replyMessage) return [...base, replyMessage]
          const local: Message = {
            id: `err-${Date.now()}`,
            sessionId: sid,
            role: 'assistant',
            agentId: agentIds[0],
            text: '',
            steps: JSON.stringify([{ kind: 'error', text: detail, reason: 'client_error' }]),
            createdAt: Math.floor(Date.now() / 1000),
          }
          return [...base, local]
        })
      }
      try {
        await api.chatStream(sid, text, agentIds, {
          attachments,
          onMeta: (m) => {
            const h = runsRef.current.get(sid)
            if (h) h.runId = m.runId
            onSid((prev) =>
              prev.map((x) => (x.id === optimistic.id ? m.userMessage : x)),
            )
          },
          onAgentStart: (a) => {
            liveId = `live-${a.index}-${Date.now()}`
            liveSteps = []
            const bubble: Message = {
              id: liveId,
              sessionId: sid,
              role: 'assistant',
              agentId: a.agentId,
              text: '',
              steps: '[]',
              createdAt: Math.floor(Date.now() / 1000),
            }
            onSid((prev) => [...prev, bubble])
            setPendingSessions((p) => withRemoved(p, sid))
          },
          onStep: (st) => {
            const id = liveId
            // Interactive prompt: the agent paused on ask_user. Surface the
            // question (transient — not added to the persisted trace); the user's
            // answer resumes the turn over the same stream.
            if (st.kind === 'ask') {
              setPendingAsks((p) => ({ ...p, [sid]: { question: st.text || '', options: st.options, kind: 'ask' } }))
              return
            }
            // Permission gate: the agent paused waiting for approval of a
            // write/exec tool. Surface the approval card (transient); the user's
            // decision resumes the turn over the same answer channel as ask_user.
            if (st.kind === 'permission') {
              setPendingAsks((p) => ({
                ...p,
                [sid]: { question: '', options: st.options, kind: 'permission', tool: st.tool, risk: st.reason, cmd: st.text },
              }))
              return
            }
            // Streaming providers emit incremental "delta" steps: append the
            // chunk to the live bubble's text instead of the activity trace.
            if (st.kind === 'delta') {
              const chunk = st.text || ''
              onSid((prev) =>
                prev.map((x) => (x.id === id ? { ...x, text: x.text + chunk } : x)),
              )
              return
            }
            // Tombstone: retract a previously emitted live step by id.
            if (st.kind === 'tombstone') {
              liveSteps = liveSteps.filter((s) => s.id !== st.ref)
            } else if (st.kind === 'thinking' && st.id) {
              // Merge streamed reasoning chunks into one growing thinking block.
              const idx = liveSteps.findIndex((s) => s.kind === 'thinking' && s.id === st.id)
              if (idx >= 0) {
                const merged = { ...liveSteps[idx], text: (liveSteps[idx].text || '') + (st.text || '') }
                liveSteps = liveSteps.map((s, k) => (k === idx ? merged : s))
              } else {
                liveSteps = [...liveSteps, st]
              }
            } else if (st.kind === 'tool_delta' && st.id) {
              // Merge streaming tool output into the existing chunk of the same id.
              const idx = liveSteps.findIndex((s) => s.kind === 'tool_delta' && s.id === st.id)
              if (idx >= 0) {
                const merged = { ...liveSteps[idx], output: (liveSteps[idx].output || '') + (st.output || '') }
                liveSteps = liveSteps.map((s, k) => (k === idx ? merged : s))
              } else {
                liveSteps = [...liveSteps, st]
              }
            } else {
              liveSteps = [...liveSteps, st]
            }
            const json = JSON.stringify(liveSteps)
            onSid((prev) =>
              prev.map((x) => (x.id === id ? { ...x, steps: json } : x)),
            )
          },
          onReply: (r) => {
            const id = liveId
            setPendingAsks((p) => withoutKey(p, sid))
            onSid((prev) => prev.map((x) => (x.id === id ? r.replyMessage : x)))
            // Clicking the notification jumps to the source chat session.
            notify(notifyEnabled.current, 'SwarmGo — yanıt hazır', r.replyMessage.text, () => {
              setView('chat')
              selectSession(sid)
            }, `chat-reply:${sid}:${r.replyMessage.id}`)
          },
          onDone: (d) => {
            void d
            bumpMeter()
            // Clear the unread flag only if the user is still viewing this turn's
            // session; otherwise leave it unread (the reply landed off-screen) and
            // just reload the list to refresh order/times/counts + the unread dot.
            if (activeSessionIdRef.current === sid) {
              api.markSessionRead(sid).catch(() => {}).finally(refreshSessions)
            } else {
              refreshSessions()
            }
          },
          onError: (err, replyMessage) => {
            // Surface the failure inline in the transcript (hierarchy) instead of
            // a top banner. Clicking the notification jumps to this chat.
            renderTurnError(err, replyMessage)
            notify(notifyEnabled.current, 'SwarmGo — hata', err, () => {
              setView('chat')
              selectSession(sid)
            }, `chat-error:${sid}:${liveId}`)
          },
        }, ac.signal, thinkingLevel, permissionMode)
      } catch (e) {
        // A deliberate stop/interrupt aborts the fetch: keep the partial reply
        // bubble visible and don't surface it as an error.
        if (!ac.signal.aborted) {
          const msg = (e as Error).message
          // Client-side/transport failure: show it inline in the transcript.
          renderTurnError(msg)
          notify(notifyEnabled.current, 'SwarmGo — hata', msg, () => {
            setView('chat')
            selectSession(sid)
          }, `chat-error:${sid}:${liveId}`)
        }
      } finally {
        // Tear down only THIS session's streaming state. Overlapping turns in
        // other sessions keep their own entries untouched.
        setStreamingSessions((p) => withRemoved(p, sid))
        setPendingSessions((p) => withRemoved(p, sid))
        setPendingAsks((p) => withoutKey(p, sid))
        const h = runsRef.current.get(sid)
        if (h && h.ac === ac) runsRef.current.delete(sid)
      }
    },
    [activeSessionId, sessions, thinkingLevel, permissionMode, activeSessionIdRef, notifyEnabled, setMessages, setError, setView, selectSession, refreshSessions, bumpMeter],
  )

  // Keep a live ref to sendMessage so effects (queue flush) can call the latest
  // version without listing it as a dependency or hitting TDZ.
  const sendMessageRef = useRef(sendMessage)
  useEffect(() => {
    sendMessageRef.current = sendMessage
  }, [sendMessage])

  // retryMessage re-runs the turn behind a failed assistant bubble. It finds the
  // user message that triggered the failure, removes the failed pair (locally and
  // server-side if it was persisted), and re-sends the same text + attachments.
  // Deleting the old pair first keeps history clean (no duplicate user bubble).
  const retryMessage = useCallback(
    async (failedId: string) => {
      const sid = activeSessionIdRef.current
      if (!sid) return
      const msgs = messagesRef.current ?? []
      const failedIdx = msgs.findIndex((m) => m.id === failedId)
      if (failedIdx < 0) return
      // Walk back from the failed assistant bubble to its triggering user message.
      let userIdx = -1
      for (let i = failedIdx; i >= 0; i--) {
        if (msgs[i].role === 'user') {
          userIdx = i
          break
        }
      }
      if (userIdx < 0) return
      const userMsg = msgs[userIdx]
      const text = userMsg.text
      const attachments = userMsg.attachments ?? []

      // A local-only bubble (optimistic/synthesized) has a client-side id prefix
      // and was never persisted, so it only needs removing from view.
      const isLocal = (id: string) =>
        id.startsWith('tmp-') || id.startsWith('err-') || id.startsWith('live-')
      const removeIds = [failedId, userMsg.id]
      setMessages((prev) => prev.filter((m) => !removeIds.includes(m.id)))
      for (const id of removeIds) {
        if (!isLocal(id)) await api.deleteMessage(sid, id).catch(() => {})
      }

      await sendMessageRef.current(text, sid, attachments)
    },
    [activeSessionIdRef, messagesRef, setMessages],
  )

  // When a session's turn ends, drop any of ITS still-pending steers (their
  // target run is gone) and cancel their grace timers.
  useEffect(() => {
    setQueuedItems((prev) => {
      const stale = prev.filter((p) => p.kind === 'steer' && !streamingSessions.has(p.sid))
      if (stale.length === 0) return prev
      stale.forEach((p) => {
        const t = steerTimers.current.get(p.id)
        if (t) {
          clearTimeout(t)
          steerTimers.current.delete(p.id)
        }
      })
      const staleIds = new Set(stale.map((p) => p.id))
      return prev.filter((p) => !staleIds.has(p.id))
    })
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

  // Stop: cancel the active session's in-flight turn. The turn is detached from
  // the SSE connection server-side, so aborting the fetch alone no longer stops
  // generation — send an explicit "stop" control, then close the stream.
  const stopTurn = useCallback(() => {
    const sid = activeSessionId
    if (!sid) return
    const h = runsRef.current.get(sid)
    if (h?.runId) api.chatControl(h.runId, 'stop', '').catch(() => {})
    h?.ac.abort()
    setStreamingSessions((p) => withRemoved(p, sid))
    setPendingSessions((p) => withRemoved(p, sid))
    setPendingAsks((p) => withoutKey(p, sid))
  }, [activeSessionId])

  // Answer: deliver the user's reply to the active session's turn paused on
  // ask_user, resuming it.
  const answerAsk = useCallback((text: string) => {
    const sid = activeSessionId
    if (!sid) return
    setPendingAsks((p) => withoutKey(p, sid))
    const h = runsRef.current.get(sid)
    if (h?.runId) {
      api.chatControl(h.runId, 'answer', text).catch((e) => setError((e as Error).message))
    }
  }, [activeSessionId, setError])

  // Interrupt: stop the active session's turn and immediately send a new message
  // to the SAME session.
  const interruptTurn = useCallback(
    (text: string) => {
      const sid = activeSessionId
      if (!sid) return
      const h = runsRef.current.get(sid)
      if (h?.runId) api.chatControl(h.runId, 'stop', '').catch(() => {})
      h?.ac.abort()
      setTimeout(() => void sendMessage(text, sid), 0)
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

  // Stable id for a pending item (no crypto needed — display/dedup only).
  const pendingId = () => `p-${Date.now()}-${Math.round(Math.random() * 1e6)}`

  // Queue: stage a message to auto-send (FIFO) when the ACTIVE session's current
  // turn finishes. Tagged with the session so it flushes to the right turn.
  const queueMessage = useCallback((text: string) => {
    const sid = activeSessionId
    if (!sid) return
    setQueuedItems((prev) => [...prev, { id: pendingId(), text, kind: 'queue', sid }])
  }, [activeSessionId])

  // Steer: stage live guidance with a short cancellable grace window, then POST
  // it to the ACTIVE session's running turn.
  const steerTurn = useCallback((text: string) => {
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
  }, [activeSessionId, setError])

  // Remove a staged item before it is processed.
  const removePending = useCallback((id: string) => {
    const t = steerTimers.current.get(id)
    if (t) {
      clearTimeout(t)
      steerTimers.current.delete(id)
    }
    setQueuedItems((prev) => prev.filter((p) => p.id !== id))
  }, [])

  // Run a "/" command that posts an assistant message: summary (memory/board/
  // flows/tools), reflection or conversation compaction.
  const summarize = useCallback(
    (kind: string) => {
      const sid = activeSessionId
      if (!sid) return
      const now = Math.floor(Date.now() / 1000)
      const userTmp = `cmd-u-${Date.now()}`
      const botTmp = `cmd-a-${Date.now()}`
      const busyLabel =
        kind === 'reflect' ? '⏳ Yansıma üretiliyor…' : kind === 'compact' ? '⏳ Sohbet sıkıştırılıyor…' : '⏳ Özetleniyor…'
      const cmdBubble: Message = { id: userTmp, sessionId: sid, role: 'user', text: '/' + kind, createdAt: now }
      const placeholder: Message = {
        id: botTmp,
        sessionId: sid,
        role: 'assistant',
        agentId: activeAgentId ?? undefined,
        text: busyLabel,
        steps: '[]',
        createdAt: now,
      }
      setMessages((prev) => [...prev, cmdBubble, placeholder])
      api
        .summarizeSession(sid, kind)
        .then(({ userMessage, replyMessage }) =>
          setMessages((prev) =>
            prev.map((m) => (m.id === userTmp ? userMessage : m.id === botTmp ? replyMessage : m)),
          ),
        )
        .catch((e) => {
          setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
          setError((e as Error).message)
        })
    },
    [activeSessionId, activeAgentId, setMessages, setError],
  )

  // Run a flow from the chat composer, streaming node-by-node progress over SSE:
  // an optimistic user bubble + a live assistant bubble whose transcript grows as
  // each node finishes, replaced by the persisted reply when the run completes.
  const runFlow = useCallback(
    (flowId: string, flowName: string, input: string, attachments: Attachment[] = []) => {
      const sid = activeSessionId
      if (!sid) return
      const now = Math.floor(Date.now() / 1000)
      const userTmp = `flow-u-${Date.now()}`
      const botTmp = `flow-a-${Date.now()}`
      const userBubble: Message = {
        id: userTmp,
        sessionId: sid,
        role: 'user',
        text: input.trim() || `🔀 ${flowName}`,
        attachments: attachments.length ? attachments : undefined,
        createdAt: now,
      }
      const placeholder: Message = {
        id: botTmp,
        sessionId: sid,
        role: 'assistant',
        agentId: activeAgentId ?? undefined,
        text: `🔀 **${flowName}**\n\n_⏳ başlatılıyor…_`,
        steps: '[]',
        createdAt: now,
      }
      setMessages((prev) => [...prev, userBubble, placeholder])

      // Live transcript: each node shows a spinner until its output arrives,
      // re-assembled on every event into the bubble's markdown. Keyed by nodeId
      // (parallel children share an execution index, so index can't be the key);
      // ordered by arrival so parallel nodes list in a stable order.
      const nodes = new Map<string, { title: string; output?: string; error?: string }>()
      const render = () => {
        let s = `🔀 **${flowName}**\n\n`
        let i = 0
        for (const n of nodes.values()) {
          i++
          const body = n.error !== undefined ? `⚠️ ${n.error}` : (n.output ?? '_⏳ çalışıyor…_')
          s += `#### ${i}. ${n.title}\n\n${body}\n\n`
        }
        return s.trim()
      }
      const setBotText = (text: string) =>
        setMessages((prev) => prev.map((m) => (m.id === botTmp ? { ...m, text } : m)))

      api
        .runFlowStream(sid, flowId, input, {
          attachments,
          onMeta: ({ userMessage }) =>
            setMessages((prev) => prev.map((m) => (m.id === userTmp ? userMessage : m))),
          onNode: (ev) => {
            const cur = nodes.get(ev.nodeId) ?? { title: ev.title }
            cur.title = ev.title
            if (ev.phase === 'done') cur.output = ev.output ?? ''
            else if (ev.phase === 'error') cur.error = ev.error ?? 'hata'
            nodes.set(ev.nodeId, cur)
            setBotText(render())
          },
          onReply: ({ replyMessage }) =>
            setMessages((prev) => prev.map((m) => (m.id === botTmp ? replyMessage : m))),
          onError: (err) => {
            setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
            setError(err)
          },
        })
        .catch((e) => {
          setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
          setError((e as Error).message)
        })
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
  // commands plus one entry per flow (🔀, takes the rest of the line as input).
  const chatCommands = useMemo<SlashCommand[]>(
    () => [
      { name: 'reflect', icon: '✦', description: 'Ajana yansıma (dream cycle) ürettir', run: () => summarize('reflect') },
      { name: 'compact', icon: '🗜', description: 'Sohbeti şimdi özete sıkıştır', run: () => summarize('compact') },
      { name: 'memory', icon: '⛁', description: 'Hafıza kayıtlarını özetle', run: () => summarize('memory') },
      { name: 'tools', icon: '🔌', description: 'Kullanılabilir araçları listele', run: () => summarize('tools') },
      { name: 'board', icon: '🗂', description: 'Görev panosunu özetle', run: () => summarize('board') },
      { name: 'flows', icon: '🔀', description: 'Akışları özetle', run: () => summarize('flows') },
      ...flows.map(
        (f): SlashCommand => ({
          name: flowSlug(f.name) || f.id.slice(0, 6),
          icon: '🔀',
          description: f.description ? `${f.name} — ${f.description}` : `${f.name} akışını çalıştır`,
          takesInput: true,
          run: (input?: string, attachments?: Attachment[]) =>
            runFlow(f.id, f.name, input ?? '', attachments ?? []),
        }),
      ),
    ],
    [summarize, flows, runFlow],
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
    stopTurn,
    answerAsk,
    interruptTurn,
    queueMessage,
    steerTurn,
    removePending,
    markPending,
    clearPending,
    setWakeWait,
    clearWakeWait,
    cancelWake,
    summarize,
    chatCommands,
    activeStreaming,
    activePending,
    activeAsk,
    activeWakeWait,
    activeQueued,
  }
}

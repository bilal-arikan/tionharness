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
import type { Agent, Message, Session, SlashCommand, TurnStep } from '../types'
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

export interface ChatStreamDeps {
  agents: Agent[]
  sessions: Session[]
  activeSessionId: string | null
  activeAgentId: string | null
  // Ref to the active session id so the detached SSE callbacks can tell whether
  // an update belongs to the session currently on screen.
  activeSessionIdRef: RefObject<string | null>
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
    agents,
    sessions,
    activeSessionId,
    activeAgentId,
    activeSessionIdRef,
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

  const sendMessage = useCallback(
    // `targetSid` lets a queued-message flush (or interrupt) send to a specific
    // session even if the user has since switched away; defaults to the active
    // session for normal sends.
    async (text: string, targetSid?: string) => {
      const sid = targetSid ?? activeSessionId
      if (!sid) return
      setError(null)

      // Resolve "@mentions" → ordered agentIds. No mention → the session's
      // default agent answers; multiple → each answers in order.
      const mentioned: string[] = []
      const norm = (v: string) => v.toLowerCase().replace(/\s+/g, '')
      const re = /(?:^|\s)@([^\s@]+)/g
      let mm: RegExpExecArray | null
      while ((mm = re.exec(text)) !== null) {
        const q = norm(mm[1])
        const a =
          agents.find((ag) => norm(ag.name) === q) ??
          agents.find((ag) => norm(ag.name).startsWith(q))
        if (a && !mentioned.includes(a.id)) mentioned.push(a.id)
      }
      const sessAgent = sessions.find((s) => s.id === sid)?.agentId
      const agentIds = mentioned.length ? mentioned : sessAgent ? [sessAgent] : []

      const now = Math.floor(Date.now() / 1000)
      const optimistic: Message = {
        id: `tmp-${Date.now()}`,
        sessionId: sid,
        role: 'user',
        text,
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
      setPendingSessions((p) => withAdded(p, sid))
      const ac = new AbortController()
      runsRef.current.set(sid, { runId: '', ac })
      setStreamingSessions((p) => withAdded(p, sid))

      // The live bubble for the agent currently answering (multi-agent turns
      // produce several bubbles, one per agent, in order).
      let liveId = ''
      let liveSteps: TurnStep[] = []
      try {
        await api.chatStream(sid, text, agentIds, {
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
              setPendingAsks((p) => ({ ...p, [sid]: { question: st.text || '', options: st.options } }))
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
            })
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
          onError: (err) => {
            setError(err)
            setPendingAsks((p) => withoutKey(p, sid))
            onSid((prev) =>
              prev.filter((m) => !m.id.startsWith('live-') && m.id !== optimistic.id),
            )
            // Clicking the notification jumps to the logs view to inspect it.
            notify(notifyEnabled.current, 'SwarmGo — hata', err, () => setView('logs'))
          },
        }, ac.signal, thinkingLevel, permissionMode)
      } catch (e) {
        // A deliberate stop/interrupt aborts the fetch: keep the partial reply
        // bubble visible and don't surface it as an error.
        if (!ac.signal.aborted) {
          const msg = (e as Error).message
          setError(msg)
          onSid((prev) =>
            prev.filter((m) => !m.id.startsWith('live-') && m.id !== optimistic.id),
          )
          notify(notifyEnabled.current, 'SwarmGo — hata', msg, () => setView('logs'))
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
    [activeSessionId, agents, sessions, thinkingLevel, permissionMode, activeSessionIdRef, notifyEnabled, setMessages, setError, setView, selectSession, refreshSessions, bumpMeter],
  )

  // Keep a live ref to sendMessage so effects (queue flush) can call the latest
  // version without listing it as a dependency or hitting TDZ.
  const sendMessageRef = useRef(sendMessage)
  useEffect(() => {
    sendMessageRef.current = sendMessage
  }, [sendMessage])

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

  // Slash commands available in the chat composer ("/" menu).
  const chatCommands = useMemo<SlashCommand[]>(
    () => [
      { name: 'reflect', icon: '✦', description: 'Ajana yansıma (dream cycle) ürettir', run: () => summarize('reflect') },
      { name: 'compact', icon: '🗜', description: 'Sohbeti şimdi özete sıkıştır', run: () => summarize('compact') },
      { name: 'memory', icon: '⛁', description: 'Hafıza kayıtlarını özetle', run: () => summarize('memory') },
      { name: 'tools', icon: '🔌', description: 'Kullanılabilir araçları listele', run: () => summarize('tools') },
      { name: 'board', icon: '🗂', description: 'Görev panosunu özetle', run: () => summarize('board') },
      { name: 'flows', icon: '🔀', description: 'Akışları özetle', run: () => summarize('flows') },
    ],
    [summarize],
  )

  // Derive the active session's view of the per-session streaming state.
  const activeStreaming = activeSessionId ? streamingSessions.has(activeSessionId) : false
  const activePending = activeSessionId ? pendingSessions.has(activeSessionId) : false
  const activeAsk = activeSessionId ? pendingAsks[activeSessionId] ?? null : null
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
    stopTurn,
    answerAsk,
    interruptTurn,
    queueMessage,
    steerTurn,
    removePending,
    summarize,
    chatCommands,
    activeStreaming,
    activePending,
    activeAsk,
    activeQueued,
  }
}

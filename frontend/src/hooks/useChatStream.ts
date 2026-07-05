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
import type {
  Agent,
  Attachment,
  Flow,
  InflightSnapshot,
  Message,
  Session,
  SlashCommand,
  TurnStep,
} from '../types'
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
      // produce several bubbles, one per agent, in order). Text and steps are
      // accumulated in closure vars so the bubble can be rebuilt in full at any
      // moment — this is what makes syncLive an UPSERT (self-healing) instead of a
      // map-only update that silently drops frames when the bubble was wiped.
      let liveId = ''
      let liveSteps: TurnStep[] = []
      let liveText = ''
      let liveAgentId = agentIds[0]
      let liveCreatedAt = 0

      // composeLive rebuilds the current live bubble from the accumulated state.
      const composeLive = (): Message => ({
        id: liveId,
        sessionId: sid,
        role: 'assistant',
        agentId: liveAgentId,
        text: liveText,
        steps: JSON.stringify(liveSteps),
        createdAt: liveCreatedAt,
      })
      // syncLive pushes the current live bubble into the transcript (UPSERT: map if
      // present, append if the bubble was wiped by a session-switch reload) and
      // mirrors it into liveBubblesRef so reseedLive can restore it on return. The
      // ref is updated even when this session is off-screen (onSid no-ops there),
      // so switching back re-seeds the freshest bubble.
      const syncLive = () => {
        if (!liveId) return
        const bubble = composeLive()
        liveBubblesRef.current.set(sid, bubble)
        onSid((prev) =>
          prev.some((x) => x.id === liveId)
            ? prev.map((x) => (x.id === liveId ? bubble : x))
            : [...prev, bubble],
        )
      }

      // Render a turn failure INTO the transcript (not just a top banner) so the
      // error is visible in the message hierarchy with its detail and survives a
      // reload when the server persisted it. The user message is kept; the live
      // (unsaved) bubble is replaced by the error bubble. When the server sent a
      // persisted error message (replyMessage) we show that; otherwise — a
      // client-side/transport failure — we synthesize a local error bubble.
      const renderTurnError = (detail: string, replyMessage?: Message) => {
        setPendingAsks((p) => withoutKey(p, sid))
        liveBubblesRef.current.delete(sid)
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
            liveText = ''
            liveAgentId = a.agentId
            liveCreatedAt = Math.floor(Date.now() / 1000)
            syncLive()
            setPendingSessions((p) => withRemoved(p, sid))
          },
          onStep: (st) => {
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
            // Plan gate: a claude-cli agent called ExitPlanMode and is waiting for
            // the user to approve its plan. Surface the plan-approval card
            // (transient); the decision resumes the turn over the same channel.
            if (st.kind === 'plan') {
              setPendingAsks((p) => ({
                ...p,
                [sid]: { question: '', options: st.options, kind: 'plan', cmd: st.text },
              }))
              return
            }
            // Streaming providers emit incremental "delta" steps: append the
            // chunk to the live bubble's text instead of the activity trace.
            // syncLive UPSERTs, so a delta arriving after a session-switch reload
            // wiped the bubble re-creates it (with the full accumulated text)
            // instead of being silently dropped.
            if (st.kind === 'delta') {
              liveText += st.text || ''
              syncLive()
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
            syncLive()
          },
          onReply: (r) => {
            setPendingAsks((p) => withoutKey(p, sid))
            // The reply persists under a DIFFERENT id (the server's replyId) than the
            // synthetic live-* bubble. Drop the live bubble (and any stale copy of the
            // persisted one) and append the authoritative message — appending, not
            // map, so it survives a reload that wiped the live bubble. The live entry
            // is no longer "unpersisted", so clear it from the ref.
            liveBubblesRef.current.delete(sid)
            onSid((prev) => {
              const without = prev.filter((x) => x.id !== liveId && x.id !== r.replyMessage.id)
              return [...without, r.replyMessage]
            })
            // Clicking the notification jumps to the source chat session.
            notify(notifyEnabled.current, 'TionSwarm — yanıt hazır', r.replyMessage.text, () => {
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
            notify(notifyEnabled.current, 'TionSwarm — hata', err, () => {
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
          notify(notifyEnabled.current, 'TionSwarm — hata', msg, () => {
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
        liveBubblesRef.current.delete(sid)
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

  // Rewind ("/rewind"): conversation-only checkpoint restore. The command opens a
  // picker (rewindOpen) listing the session's user prompts; choosing one truncates
  // the transcript back to that checkpoint. File changes are NOT reverted.
  const [rewindOpen, setRewindOpen] = useState(false)
  const openRewind = useCallback(() => setRewindOpen(true), [])
  const closeRewind = useCallback(() => setRewindOpen(false), [])

  // rewindTo removes the anchor message and everything after it, from the view
  // and (atomically) server-side. Returns the removed prompt text so the caller
  // can drop it back into the composer for a clean re-try. A local-only anchor
  // (never persisted) is sliced from view without a server call.
  const rewindTo = useCallback(
    async (messageId: string): Promise<string> => {
      const sid = activeSessionIdRef.current
      if (!sid) return ''
      const msgs = messagesRef.current ?? []
      const idx = msgs.findIndex((m) => m.id === messageId)
      if (idx < 0) return ''
      const promptText = msgs[idx].role === 'user' ? msgs[idx].text : ''
      setMessages((prev) => {
        const i = prev.findIndex((m) => m.id === messageId)
        return i < 0 ? prev : prev.slice(0, i)
      })
      const isLocal = (id: string) =>
        id.startsWith('tmp-') || id.startsWith('err-') || id.startsWith('live-')
      if (!isLocal(messageId)) {
        await api.rewindSession(sid, messageId).catch(() => {})
      }
      setRewindOpen(false)
      return promptText
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

  // ---- autonomous / other-window live turns (session-step bus) ----

  // A synthetic "ghost" assistant bubble per session, grown from `session_step`
  // bus frames. It lets a window that does NOT own the running turn — a scheduled/
  // spawned/worker/wake turn, or the same chat turn viewed in another window —
  // render thinking/tool steps live, exactly like the locally-streamed bubble.
  // Replaced by the authoritative persisted message when the turn ends (the
  // completion event reloads the transcript). Keyed by session because turns are
  // detached and several can overlap.
  const autoLiveRef = useRef<Map<string, { id: string; steps: TurnStep[] }>>(new Map())

  // applyAutoStep folds one bus step into the active session's ghost bubble. It is
  // a no-op when THIS window owns the turn (runsRef has it → the local per-request
  // SSE already renders it, so the bus copy would duplicate), and only mutates the
  // transcript for the session currently on screen (off-screen turns just raise the
  // thinking indicator and reload their transcript on completion).
  const applyAutoStep = useCallback(
    (sid: string, step: TurnStep) => {
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
    },
    [activeSessionIdRef, setMessages],
  )

  // clearAutoLive drops a session's ghost bubble + its accumulator once the turn
  // has ended. The caller reloads the transcript right after, so the authoritative
  // persisted message takes the ghost's place.
  const clearAutoLive = useCallback(
    (sid: string) => {
      if (!autoLiveRef.current.has(sid)) return
      const entry = autoLiveRef.current.get(sid)!
      autoLiveRef.current.delete(sid)
      // Drop the ghost bubble whether it was synthesized (live-auto-<sid>) or seeded
      // from an inflight snapshot (id = the pre-allocated reply id).
      setMessages((prev) => prev.filter((m) => m.id !== entry.id && !m.id.startsWith(`live-auto-${sid}`)))
    },
    [setMessages],
  )

  // recoverInflight restores the in-progress assistant bubble after a MID-TURN
  // reload. The turn keeps running detached server-side, but a fresh page only
  // loads persisted messages — so the steps/agent produced before the reload
  // vanish until the turn ends. This fetches the streaming snapshot (agent + text
  // + steps-so-far, written to the crash sidecar on a throttle) and seeds it as the
  // session's ghost bubble, which the session-step bus then keeps growing live. The
  // authoritative message (same id) replaces it on completion. No-op when this
  // window owns the run (its own SSE renders it) or the reply already persisted.
  const recoverInflight = useCallback(
    async (sid: string, loadedMsgs: Message[]) => {
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
    },
    [activeSessionIdRef, setMessages],
  )

  // reseedLive restores the live (unpersisted) assistant bubble THIS window is
  // streaming after a session-switch reload wiped `messages` to the persisted-only
  // list. It is the owning-window counterpart to recoverInflight: recoverInflight
  // handles the NON-owning path (seeds a ghost from the server snapshot + bus),
  // and early-returns when this window owns the run — so without reseedLive the
  // owned bubble would stay gone until the next SSE frame re-created it. Uses the
  // freshest bubble the local SSE handlers mirror into liveBubblesRef. No-op when
  // no live bubble is owned, the user switched away, or it is already present.
  const reseedLive = useCallback(
    (sid: string, loadedMsgs: Message[]) => {
      const bubble = liveBubblesRef.current.get(sid)
      if (!bubble) return
      if (activeSessionIdRef.current !== sid) return
      if (loadedMsgs.some((m) => m.id === bubble.id)) return
      setMessages((prev) => (prev.some((m) => m.id === bubble.id) ? prev : [...prev, bubble]))
    },
    [activeSessionIdRef, setMessages],
  )

  // ---- inflight text polling (non-owning / post-refresh observer) ----

  // The session-step bus deliberately DROPS delta/tool_delta frames (see
  // sessionstep.go busForwardable) to avoid flooding the shared bus with token
  // chunks — so a ghost bubble (recoverInflight-seeded, bus-grown) shows the
  // activity trace live but its ANSWER TEXT freezes at the snapshot taken when the
  // page reloaded. This poll advances just that text: while the ACTIVE session has
  // a running turn THIS window does not own, re-fetch the inflight snapshot every
  // second and fold its (throttled, ≤600ms-fresh) text into the ghost bubble. Only
  // `text` is touched — `steps` stays owned by the bus (applyAutoStep), so the two
  // never fight. Stops the instant the turn ends (pending clears → transcript
  // reload swaps in the authoritative message) or this window takes over the run.
  useEffect(() => {
    const sid = activeSessionId
    if (!sid) return
    if (!pendingSessions.has(sid)) return
    if (runsRef.current.has(sid)) return // this window owns the turn — its own SSE streams the text
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

  // Run a "/" command that posts an assistant message: summary (board/flows/
  // tools) or conversation compaction.
  const summarize = useCallback(
    (kind: string) => {
      const sid = activeSessionId
      if (!sid) return
      const now = Math.floor(Date.now() / 1000)
      const userTmp = `cmd-u-${Date.now()}`
      const botTmp = `cmd-a-${Date.now()}`
      const busyLabel =
        kind === 'compact' ? '⏳ Sohbet sıkıştırılıyor…' : '⏳ Özetleniyor…'
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

  // Context reset (/handoff): write a handoff artifact for the current session and
  // spawn a fresh one to continue in a clean window, then switch the UI to it. The
  // old session keeps a tombstone linking forward; the new session opens with the
  // handoff inline.
  const handoff = useCallback(() => {
    const sid = activeSessionId
    if (!sid) return
    const now = Math.floor(Date.now() / 1000)
    const userTmp = `cmd-u-${Date.now()}`
    const botTmp = `cmd-a-${Date.now()}`
    const cmdBubble: Message = { id: userTmp, sessionId: sid, role: 'user', text: '/handoff', createdAt: now }
    const placeholder: Message = {
      id: botTmp,
      sessionId: sid,
      role: 'assistant',
      agentId: activeAgentId ?? undefined,
      text: '⏳ Context reset — handoff yazılıyor ve temiz oturum başlatılıyor…',
      steps: '[]',
      createdAt: now,
    }
    setMessages((prev) => [...prev, cmdBubble, placeholder])
    api
      .handoffSession(sid)
      .then(({ newSessionId }) => {
        // Refresh the list (old tombstone + new session) and jump to the fresh one.
        refreshSessions()
        if (newSessionId) selectSession(newSessionId)
      })
      .catch((e) => {
        setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
        setError((e as Error).message)
      })
  }, [activeSessionId, activeAgentId, setMessages, setError, refreshSessions, selectSession])

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
      { name: 'compact', icon: '🗜', description: 'Sohbeti şimdi özete sıkıştır', run: () => summarize('compact') },
      { name: 'handoff', icon: '↪', description: 'Context reset — temiz pencerede devam et', run: () => handoff() },
      { name: 'rewind', icon: '⟲', description: 'Sohbeti bir checkpoint\'e geri sar — mesajları geri al', run: () => openRewind() },
      { name: 'tools', icon: '🔌', description: 'Kullanılabilir araçları listele', run: () => summarize('tools') },
      { name: 'board', icon: '🗂', description: 'Görev panosunu özetle', run: () => summarize('board') },
      { name: 'flows', icon: '🔀', description: 'Akışları özetle', run: () => summarize('flows') },
      ...flows.map(
        (f): SlashCommand => ({
          name: flowSlug(f.name) || f.id.slice(0, 6),
          icon: '🔀',
          description: `${f.name} akışını çalıştır`,
          takesInput: true,
          run: (input?: string, attachments?: Attachment[]) =>
            runFlow(f.id, f.name, input ?? '', attachments ?? []),
        }),
      ),
    ],
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

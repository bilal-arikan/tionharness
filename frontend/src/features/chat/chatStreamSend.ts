// Send/stream loop: performSend drives one chat turn end-to-end over SSE —
// optimistic user bubble, live assistant bubble upkeep (deltas/steps/asks),
// reply/error handling and per-session streaming-state teardown. Extracted
// from useChatStream as a plain function; the hook's sendMessage callback
// builds the context and delegates here.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import { api } from '@/api'
import type { Attachment, Message, Session, TurnStep } from '@/types'
import type { View } from '@/app/NavRail'
import type { PendingAsk } from './AskPrompt'
import { notify } from '@/shared/lib/clientPrefs'
import { withAdded, withRemoved, withoutKey } from './chatStreamHelpers'
import type { RunHandle, WakeWait } from './chatStreamTypes'

export interface SendContext {
  activeSessionId: string | null
  sessions: Session[]
  thinkingLevel: string
  permissionMode: string
  activeSessionIdRef: RefObject<string | null>
  notifyEnabled: RefObject<boolean>
  runsRef: RefObject<Map<string, RunHandle>>
  liveBubblesRef: RefObject<Map<string, Message>>
  setMessages: Dispatch<SetStateAction<Message[]>>
  setStreamingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setPendingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setPendingAsks: Dispatch<SetStateAction<Record<string, PendingAsk>>>
  setWakeWaits: Dispatch<SetStateAction<Record<string, WakeWait>>>
  setError: (msg: string | null) => void
  setView: (v: View) => void
  selectSession: (id: string) => void
  refreshSessions: () => void
  bumpMeter: () => void
}

// `targetSid` lets a queued-message flush (or interrupt) send to a specific
// session even if the user has since switched away; defaults to the active
// session for normal sends.
export async function performSend(
  ctx: SendContext,
  text: string,
  targetSid: string | undefined,
  attachments: Attachment[],
): Promise<void> {
  const {
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
  } = ctx
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
}

// Send/stream loop: performSend drives one chat turn end-to-end over SSE —
// optimistic user bubble, live assistant bubble upkeep (deltas/steps/asks),
// reply/error handling and per-session streaming-state teardown. Extracted
// from useChatStream as a plain function; the hook's sendMessage callback
// builds the context and delegates here.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import { api } from '@/api'
import type { Attachment, Message, Session } from '@/types'
import type { View } from '@/app/NavRail'
import type { PendingAsk } from './AskPrompt'
import type { PendingItem } from './PendingTray'
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
  setQueued: Dispatch<SetStateAction<PendingItem[]>>
  setPendingAsks: Dispatch<SetStateAction<Record<string, PendingAsk>>>
  setWakeWaits: Dispatch<SetStateAction<Record<string, WakeWait>>>
  setError: (msg: string | null) => void
  setView: (v: View) => void
  selectSession: (id: string) => void
  refreshSessions: () => void
  bumpMeter: () => void
}

// Faz 3 (_Docs/58): a send is now an ENQUEUE. The message goes to the session's
// durable backend queue (idempotent on clientMsgId); the serial worker runs it in
// order and the hub streams the turn to every window. So performSend paints
// nothing — no optimistic bubble: a queued message shows in the tray (queue_update)
// and becomes a chat bubble when the worker runs it (user_message). `targetSid`
// lets an interrupt target a specific session even if the user switched away.
export async function performSend(
  ctx: SendContext,
  text: string,
  targetSid: string | undefined,
  attachments: Attachment[],
): Promise<void> {
  const { activeSessionId, sessions, thinkingLevel, permissionMode, setPendingSessions, setQueued, setWakeWaits, setError } = ctx
  const sid = targetSid ?? activeSessionId
  if (!sid) return
  setError(null)
  // One agent per turn: the session's bound agent answers.
  const sessAgent = sessions.find((s) => s.id === sid)?.agentId
  const agentIds = sessAgent ? [sessAgent] : []
  const clientMsgId = genClientMsgId()
  // A fresh message supersedes any pending self-wake banner.
  setWakeWaits((p) => withoutKey(p, sid))
  // Instant "accepted" feedback; the hub takes over from here.
  setPendingSessions((p) => withAdded(p, sid))
  // Optimistic queue chip (only for the on-screen session): show the message in
  // the tray the instant it is submitted, before the server round-trip. The
  // server's queue_update — carrying the SAME clientMsgId — reconciles it (kept
  // while it waits, dropped when the worker runs it and a real bubble appears).
  if (sid === activeSessionId) {
    setQueued((prev) => (prev.some((p) => p.id === clientMsgId) ? prev : [...prev, { id: clientMsgId, text, kind: 'queue', sid }]))
  }
  try {
    await api.enqueueMessage(sid, { message: text, agentIds, attachments, thinkingLevel, permissionMode, clientMsgId })
  } catch (e) {
    setPendingSessions((p) => withRemoved(p, sid))
    setQueued((prev) => prev.filter((p) => p.id !== clientMsgId))
    setError((e as Error).message)
  }
}

// genClientMsgId returns a unique id for a submitted message, used for idempotent
// enqueue (dedupes double-submits / retries / reconnect replays).
function genClientMsgId(): string {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto
  if (c?.randomUUID) return c.randomUUID()
  return `cmid-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

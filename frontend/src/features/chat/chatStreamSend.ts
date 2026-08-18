// Send path: performSend submits one user turn to the session's durable backend
// queue. It paints NOTHING — rendering is the hub subscription's job (see
// chatStreamHub). Extracted from useChatStream as a plain function; the hook's
// sendMessage callback builds the context and delegates here.
import type { Dispatch, SetStateAction } from 'react'
import { api } from '@/api'
import type { Attachment, Session } from '@/types'
import type { PendingItem } from './PendingTray'
import { withAdded, withRemoved, withoutKey } from './chatStreamHelpers'
import type { WakeWait } from './chatStreamTypes'

// Exactly what performSend touches — nothing more. This interface used to mirror
// the whole chat hook (run handles, live bubbles, transcript setters, navigation)
// because performSend once painted the turn itself; the Faz 3 queue cutover made
// all of that the hub's job, but the dead fields lingered. They were not harmless:
// a `bumpMeter` that was threaded here and never called is exactly why the Session
// Info panel silently stopped refreshing. Keep this list minimal.
export interface SendContext {
  activeSessionId: string | null
  sessions: Session[]
  thinkingLevel: string
  permissionMode: string
  setPendingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setQueued: Dispatch<SetStateAction<PendingItem[]>>
  setWakeWaits: Dispatch<SetStateAction<Record<string, WakeWait>>>
  setError: (msg: string | null) => void
}

// Faz 3 (_Docs/58): a send is now an ENQUEUE. The message goes to the session's
// durable backend queue (idempotent on clientMsgId); the serial worker runs it in
// order and the hub streams the turn to every window. So performSend paints
// nothing — no optimistic bubble: a queued message shows in the tray (queue_update)
// and becomes a chat bubble when the worker runs it (user_message). `targetSid`
// lets an interrupt target a specific session even if the user switched away.
// Returns true when the enqueue was accepted by the backend, false when it
// failed (or there was no session to send to). The composer uses this to decide
// whether to clear its input: a failed send keeps the text so it can be retried.
export async function performSend(
  ctx: SendContext,
  text: string,
  targetSid: string | undefined,
  attachments: Attachment[],
): Promise<boolean> {
  const {
    activeSessionId,
    sessions,
    thinkingLevel,
    permissionMode,
    setPendingSessions,
    setQueued,
    setWakeWaits,
    setError,
  } = ctx
  const sid = targetSid ?? activeSessionId
  if (!sid) return false
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
    // An attachment-only turn has no text; label the chip by its files so the
    // tray shows something the user can recognise (and cancel).
    const chipText = text.trim() ? text : `${attachments.length} ek`
    setQueued((prev) =>
      prev.some((p) => p.id === clientMsgId)
        ? prev
        : [...prev, { id: clientMsgId, text: chipText, kind: 'queue', sid }],
    )
  }
  try {
    await api.enqueueMessage(sid, {
      message: text,
      agentIds,
      attachments,
      thinkingLevel,
      permissionMode,
      clientMsgId,
    })
    return true
  } catch (e) {
    setPendingSessions((p) => withRemoved(p, sid))
    setQueued((prev) => prev.filter((p) => p.id !== clientMsgId))
    setError((e as Error).message)
    return false
  }
}

// genClientMsgId returns a unique id for a submitted message, used for idempotent
// enqueue (dedupes double-submits / retries / reconnect replays).
function genClientMsgId(): string {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto
  if (c?.randomUUID) return c.randomUUID()
  return `cmid-${Date.now()}-${Math.random().toString(36).slice(2)}`
}

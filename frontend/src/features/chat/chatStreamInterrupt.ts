// Interrupt path: performInterrupt cuts the running turn short and takes the
// session's next turn slot in ONE server call (POST control {action:"interrupt"}).
//
// It exists separately from performSend because the two-call version it replaces
// (sessionControl('stop') then sendMessage) had a race that no amount of client
// sequencing could close: between the two round trips the session's turn slot is
// free, and another queued message or an autonomous turn (worker notification,
// self-wake, schedule) could claim it — so the "send now" message ran after the
// very turn the user interrupted to get ahead of.
//
// Like performSend it paints NOTHING beyond the optimistic queue chip: rendering
// is the hub subscription's job.
import { api } from '@/api'
import type { Attachment } from '@/types'
import { withAdded, withRemoved, withoutKey } from './chatStreamHelpers'
import type { SendContext } from './chatStreamSend'
import { genClientMsgId } from './chatStreamSend'

// Returns true when the interrupt was accepted (message queued at the head).
// False lets the composer keep the draft for a retry, exactly as performSend does.
export async function performInterrupt(
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
  const sessAgent = sessions.find((s) => s.id === sid)?.agentId
  const agentIds = sessAgent ? [sessAgent] : []
  const clientMsgId = genClientMsgId()
  setWakeWaits((p) => withoutKey(p, sid))
  setPendingSessions((p) => withAdded(p, sid))
  if (sid === activeSessionId) {
    const chipText = text.trim() ? text : `${attachments.length} ek`
    setQueued((prev) =>
      prev.some((p) => p.id === clientMsgId)
        ? prev
        : [...prev, { id: clientMsgId, text: chipText, kind: 'queue', sid }],
    )
  }
  try {
    await api.interruptSession(sid, {
      text,
      clientMsgId,
      agentIds,
      thinkingLevel,
      permissionMode,
      attachments,
    })
    return true
  } catch (e) {
    setPendingSessions((p) => withRemoved(p, sid))
    setQueued((prev) => prev.filter((p) => p.id !== clientMsgId))
    setError((e as Error).message)
    return false
  }
}

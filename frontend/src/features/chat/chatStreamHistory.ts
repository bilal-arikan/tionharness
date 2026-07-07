// Transcript-history operations: retry a failed turn, re-run the last turn and
// rewind to a checkpoint. Plain functions extracted from useChatStream; the
// hook's callbacks build the context and delegate here.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import { api } from '@/api'
import type { Message } from '@/types'
import type { SendFn } from './chatStreamTypes'

export interface HistoryContext {
  activeSessionIdRef: RefObject<string | null>
  messagesRef: RefObject<Message[]>
  setMessages: Dispatch<SetStateAction<Message[]>>
  // Live ref to the hook's sendMessage so these operations always call the
  // latest version without re-binding.
  sendMessageRef: RefObject<SendFn>
}

// performRetry re-runs the turn behind a failed assistant bubble. It finds the
// user message that triggered the failure, removes the failed pair (locally and
// server-side if it was persisted), and re-sends the same text + attachments.
// Deleting the old pair first keeps history clean (no duplicate user bubble).
export async function performRetry(ctx: HistoryContext, failedId: string): Promise<void> {
  const { activeSessionIdRef, messagesRef, setMessages, sendMessageRef } = ctx
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
}

// performRerunLast re-runs the most recent turn of the active session — the
// "restart" action in the Session Info panel. It retries the last assistant turn
// (removing the old pair and re-sending its prompt) so it reuses the one true
// turn path; if no assistant turn exists yet (e.g. a turn still running with
// nothing persisted), it simply re-sends the last user message. The caller
// (panel) stops any in-flight run via the backend first, so this never
// double-runs.
export async function performRerunLast(
  ctx: Omit<HistoryContext, 'setMessages'>,
  retry: (failedId: string) => Promise<void>,
): Promise<void> {
  const { activeSessionIdRef, messagesRef, sendMessageRef } = ctx
  const sid = activeSessionIdRef.current
  if (!sid) return
  const msgs = messagesRef.current ?? []
  for (let i = msgs.length - 1; i >= 0; i--) {
    if (msgs[i].role === 'assistant') {
      await retry(msgs[i].id)
      return
    }
  }
  for (let i = msgs.length - 1; i >= 0; i--) {
    if (msgs[i].role === 'user') {
      await sendMessageRef.current(msgs[i].text, sid, msgs[i].attachments ?? [])
      return
    }
  }
}

// performRewindTo removes the anchor message and everything after it, from the
// view and (atomically) server-side. Returns the removed prompt text so the
// caller can drop it back into the composer for a clean re-try. A local-only
// anchor (never persisted) is sliced from view without a server call.
export async function performRewindTo(
  ctx: Omit<HistoryContext, 'sendMessageRef'>,
  setRewindOpen: Dispatch<SetStateAction<boolean>>,
  messageId: string,
): Promise<string> {
  const { activeSessionIdRef, messagesRef, setMessages } = ctx
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
}

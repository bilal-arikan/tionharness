// Residual helpers from the pre-hub live-turn recovery. Since the full cutover
// (_Docs/58) rendering is the session-hub subscription's job (chatStreamHub), the
// bus-ghost + inflight-snapshot recovery paths were removed. These two helpers
// remain wired into the session-open message-load (reseed) and the completion
// event (clear) as harmless no-ops — the refs they touch are no longer populated
// under the hub model, so they simply do nothing until the wiring is fully retired.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import type { Message } from '@/types'
import type { AutoLiveEntry } from './chatStreamTypes'

// clearAutoLiveEntry drops a session's (legacy) ghost bubble + accumulator. Under
// the hub model autoLiveRef is never populated, so this is a no-op.
export function clearAutoLiveEntry(
  autoLiveRef: RefObject<Map<string, AutoLiveEntry>>,
  setMessages: Dispatch<SetStateAction<Message[]>>,
  sid: string,
): void {
  if (!autoLiveRef.current.has(sid)) return
  const entry = autoLiveRef.current.get(sid)!
  autoLiveRef.current.delete(sid)
  setMessages((prev) => prev.filter((m) => m.id !== entry.id && !m.id.startsWith(`live-auto-${sid}`)))
}

// performReseedLive re-injected the owning window's live bubble after a
// session-switch reload. Under the hub model liveBubblesRef is never populated
// (performSend no longer paints), so this is a no-op.
export function performReseedLive(
  liveBubblesRef: RefObject<Map<string, Message>>,
  activeSessionIdRef: RefObject<string | null>,
  setMessages: Dispatch<SetStateAction<Message[]>>,
  sid: string,
  loadedMsgs: Message[],
): void {
  const bubble = liveBubblesRef.current.get(sid)
  if (!bubble) return
  if (activeSessionIdRef.current !== sid) return
  if (loadedMsgs.some((m) => m.id === bubble.id)) return
  setMessages((prev) => (prev.some((m) => m.id === bubble.id) ? prev : [...prev, bubble]))
}

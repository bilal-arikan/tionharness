// Shared types for the chat-stream modules. useChatStream and its per-concern
// helpers (send loop, interventions, auto-live recovery, slash commands) agree
// on these shapes; the hook re-exports ChatStreamDeps so external importers
// keep using '@/features/chat/useChatStream'.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import type { Agent, Attachment, Message, Session, TurnStep } from '@/types'
import type { View } from '@/app/NavRail'

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

// A live run handle: the server-issued runId plus the AbortController of the
// SSE fetch that owns the turn in THIS window.
export interface RunHandle {
  runId: string
  ac: AbortController
}

// Signature of the hook's sendMessage, used by modules that re-send through
// the live sendMessageRef without re-binding callbacks.
export type SendFn = (text: string, targetSid?: string, attachments?: Attachment[]) => Promise<void>

// Accumulator behind a session's ghost bubble (autonomous / other-window turns
// grown from session_step bus frames).
export interface AutoLiveEntry {
  id: string
  steps: TurnStep[]
}

// A pending self-wake (schedule_wake): the agent's reason + the fire time.
export interface WakeWait {
  reason: string
  fireAt: number
}

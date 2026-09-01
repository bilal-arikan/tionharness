// sessionStartGate decides whether the pre-first-message setup card
// (SessionStartPanel) is shown. Kept out of ChatView so the rule — which is the
// whole feature: "before the first turn, and not a moment after" — is testable
// without rendering the chat screen.

export interface StartPanelGate {
  // Read-only run log (task / flow / schedule): no composer, nothing to set up.
  readOnly: boolean
  activeSessionId: string | null
  // Session the user explicitly closed the card for (in-memory, per page load).
  dismissedSessionId: string | null
  messageCount: number
  // Transcript still loading — the count is not yet meaningful, so waiting avoids
  // flashing the card on a session that already has history.
  messagesLoading: boolean
  streaming: boolean
  // A turn was enqueued but not yet painted. This is what hides the card the
  // instant the user hits send, rather than when the reply lands.
  pending: boolean
  queuedCount: number
  // A spawned worker: its coordinator already chose both settings for it and the
  // first message is on its way in.
  isWorker: boolean
}

export function shouldShowStartPanel(g: StartPanelGate): boolean {
  if (g.readOnly || !g.activeSessionId) return false
  if (g.dismissedSessionId === g.activeSessionId) return false
  if (g.messagesLoading || g.messageCount > 0) return false
  if (g.streaming || g.pending || g.queuedCount > 0) return false
  return !g.isWorker
}

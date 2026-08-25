import type { SessionDraftReadState } from '@/shared/lib/sessionDrafts'

export interface FreshSessionCleanupInput {
  freshSessionId: string | null
  leavingSessionId: string | null
  messageCount: number
  draft: SessionDraftReadState
}

// A newly-created session is disposable only while it has neither a committed
// transcript nor meaningful composer text in the active workspace. An
// unreadable draft is preserved because absence of text cannot be established.
export function shouldDiscardFreshSession({
  freshSessionId,
  leavingSessionId,
  messageCount,
  draft,
}: FreshSessionCleanupInput): boolean {
  return (
    freshSessionId !== null &&
    freshSessionId === leavingSessionId &&
    messageCount === 0 &&
    draft.ok &&
    draft.value.trim().length === 0
  )
}

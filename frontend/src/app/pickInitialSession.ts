import type { Session } from '@/types'
import { isWritableSessionKind } from './viewRegistry'
import type { Route } from './url'

export interface PickInitialSessionInput {
  sessions: Session[]
  wantRoute: Route | null
  draftedSessionIds: Set<string>
  agentExists: (agentId: string) => boolean
}

export interface PickInitialSessionResult {
  sessionId: string | null
  agentId: string | null
}

// pickInitialSession chooses which session a freshly-loaded workspace opens on.
// Priority: an explicit deep link always wins (the user, or a cross-workspace
// nav, asked for it directly); otherwise a session holding an unsent composer
// draft is preferred over the plain most-recent session, so a returning user
// (reload, window reopen) lands back on the chat they were mid-typing instead
// of it looking discarded; otherwise fall back to the most recent writable
// session.
export function pickInitialSession({
  sessions,
  wantRoute,
  draftedSessionIds,
  agentExists,
}: PickInitialSessionInput): PickInitialSessionResult {
  const firstChat = sessions.find((s) => isWritableSessionKind(s.kind))
  let sessionId = firstChat ? firstChat.id : null
  let agentId = firstChat ? firstChat.agentId : null

  if (draftedSessionIds.size > 0) {
    const draftSession = sessions.find(
      (s) => draftedSessionIds.has(s.id) && isWritableSessionKind(s.kind),
    )
    if (draftSession) {
      sessionId = draftSession.id
      agentId = draftSession.agentId
    }
  }

  if (wantRoute) {
    if (wantRoute.view === 'chat' && wantRoute.id && sessions.some((s) => s.id === wantRoute.id)) {
      sessionId = wantRoute.id
      agentId = sessions.find((s) => s.id === wantRoute.id)?.agentId ?? agentId
    } else if (wantRoute.view === 'agents' && wantRoute.id && agentExists(wantRoute.id)) {
      agentId = wantRoute.id
    }
  }

  return { sessionId, agentId }
}

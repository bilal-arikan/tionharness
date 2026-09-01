import type { Session } from '@/types'
import type { Route } from './url'

export interface SessionListRequestToken {
  epoch: number
  queryIdentity: string
}

export function sessionListQueryIdentity(workspaceId: string | null, chips: string): string {
  return `${workspaceId ?? ''}\u0000${chips}`
}

// One epoch owns every session-list request: bootstrap, chip replacement,
// refresh and load-more. Only the latest request for the current query may
// mutate the list window, regardless of which request type resolves first.
export function createSessionListRequestGuard() {
  let epoch = 0
  return {
    begin(queryIdentity: string): SessionListRequestToken {
      return { epoch: ++epoch, queryIdentity }
    },
    isCurrent(token: SessionListRequestToken, currentQueryIdentity: string): boolean {
      return token.epoch === epoch && token.queryIdentity === currentQueryIdentity
    },
  }
}

export function mergeSelectedSession(items: Session[], selected: Session | undefined): Session[] {
  if (!selected || items.some((session) => session.id === selected.id)) return items
  return [...items, selected]
}

export function appendSessionPage(current: Session[], page: Session[]): Session[] {
  const known = new Set(current.map((session) => session.id))
  return [...current, ...page.filter((session) => !known.has(session.id))]
}

export function initialSessionLookupIDs(
  wantRoute: Route | null,
  activeSessionId: string | null,
  draftedSessionIds: Set<string>,
  limit: number,
): string[] {
  return Array.from(
    new Set([
      ...(wantRoute?.view === 'chat' && wantRoute.id ? [wantRoute.id] : []),
      ...(activeSessionId ? [activeSessionId] : []),
      ...draftedSessionIds,
    ]),
  ).slice(0, limit)
}

export function chatRouteLookupResolved(
  wantRoute: Route | null,
  page: Session[],
  exactLookupSucceeded: boolean,
): boolean {
  return (
    !wantRoute ||
    wantRoute.view !== 'chat' ||
    !wantRoute.id ||
    page.some((session) => session.id === wantRoute.id) ||
    exactLookupSucceeded
  )
}

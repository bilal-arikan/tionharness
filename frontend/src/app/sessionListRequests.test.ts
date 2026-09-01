import { describe, expect, it } from 'vitest'
import type { Session } from '@/types'
import {
  appendSessionPage,
  chatRouteLookupResolved,
  createSessionListRequestGuard,
  initialSessionLookupIDs,
  mergeSelectedSession,
  sessionListQueryIdentity,
} from './sessionListRequests'

function session(id: string): Session {
  return { id, kind: 'chat', agentId: 'AGT1', title: '', createdAt: 0, updatedAt: 0 } as Session
}

describe('session list request guard', () => {
  it('rejects stale replace and load-more responses regardless of resolution order', () => {
    const guard = createSessionListRequestGuard()
    const query = sessionListQueryIdentity('WS1', 'chat')
    const replace = guard.begin(query)
    const loadMore = guard.begin(query)

    expect(guard.isCurrent(replace, query)).toBe(false)
    expect(guard.isCurrent(loadMore, query)).toBe(true)

    const refresh = guard.begin(query)
    expect(guard.isCurrent(loadMore, query)).toBe(false)
    expect(guard.isCurrent(refresh, query)).toBe(true)
  })

  it('rejects a response after workspace or chip identity changes', () => {
    const guard = createSessionListRequestGuard()
    const oldQuery = sessionListQueryIdentity('WS1', 'chat')
    const request = guard.begin(oldQuery)
    expect(guard.isCurrent(request, sessionListQueryIdentity('WS2', 'chat'))).toBe(false)
    expect(guard.isCurrent(request, sessionListQueryIdentity('WS1', 'flow'))).toBe(false)
  })
})

describe('session list window helpers', () => {
  it('merges a selected hidden session without shifting appended page deduplication', () => {
    const hidden = session('HIDDEN')
    const current = mergeSelectedSession([session('S1')], hidden)
    expect(appendSessionPage(current, [session('S2'), hidden]).map((item) => item.id)).toEqual([
      'S1',
      'HIDDEN',
      'S2',
    ])
  })

  it('keeps the deep-link target first in a bounded exact lookup', () => {
    const ids = initialSessionLookupIDs(
      { view: 'chat', id: 'DEEP', workspaceId: 'WS1' },
      null,
      new Set(['DRAFT1', 'DRAFT2']),
      2,
    )
    expect(ids).toEqual(['DEEP', 'DRAFT1'])
  })

  it('keeps the open session ahead of draft lookups when a chip hides it', () => {
    const ids = initialSessionLookupIDs(null, 'OPEN', new Set(['DRAFT1', 'DRAFT2']), 2)
    expect(ids).toEqual(['OPEN', 'DRAFT1'])
  })

  it('does not resolve a missing deep link when its exact lookup failed', () => {
    const route = { view: 'chat', id: 'DEEP', workspaceId: 'WS1' } as const
    expect(chatRouteLookupResolved(route, [session('S1')], false)).toBe(false)
    expect(chatRouteLookupResolved(route, [session('DEEP')], false)).toBe(true)
    expect(chatRouteLookupResolved(route, [session('S1')], true)).toBe(true)
  })
})

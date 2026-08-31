import { describe, expect, it } from 'vitest'
import { sessionMatchesQuery } from './sessionSearch'
import { shouldShowSessionsLoadMore } from './sessionsLoadMore'

describe('sessionMatchesQuery', () => {
  const session = { id: 'SES2235', title: 'Haftalık plan' }

  it('matches the title', () => {
    expect(sessionMatchesQuery(session, 'plan')).toBe(true)
  })

  it('matches partial session IDs', () => {
    expect(sessionMatchesQuery(session, '223')).toBe(true)
    expect(sessionMatchesQuery({ ...session, id: 'SES223' }, '223')).toBe(true)
  })

  it('rejects unrelated queries', () => {
    expect(sessionMatchesQuery(session, 'fatura')).toBe(false)
  })

  it('matches session IDs case-insensitively', () => {
    expect(sessionMatchesQuery(session, 'ses2235')).toBe(true)
  })

  it('matches an empty query', () => {
    expect(sessionMatchesQuery(session, '   ')).toBe(true)
  })
})

const visibleState = {
  loading: false,
  hasMoreSessions: true,
  canLoadMore: true,
  query: '',
  hasActiveChipFilters: false,
  filteredSessionCount: 0,
}

describe('shouldShowSessionsLoadMore', () => {
  it('stays visible without active chip filters', () => {
    expect(shouldShowSessionsLoadMore(visibleState)).toBe(true)
  })

  it('stays visible for a filtered page with few local matches while server has more', () => {
    expect(
      shouldShowSessionsLoadMore({
        ...visibleState,
        hasActiveChipFilters: true,
        filteredSessionCount: 1,
      }),
    ).toBe(true)
  })

  it('is hidden when every session is loaded', () => {
    expect(shouldShowSessionsLoadMore({ ...visibleState, hasMoreSessions: false })).toBe(false)
  })

  it('is hidden during message search', () => {
    expect(shouldShowSessionsLoadMore({ ...visibleState, query: 'ab' })).toBe(false)
  })

  it('requires a load-more callback', () => {
    expect(shouldShowSessionsLoadMore({ ...visibleState, canLoadMore: false })).toBe(false)
  })

  it('stays available across consecutive pages without local matches until server ends', () => {
    const pageStates = [true, true, false].map((hasMoreSessions) =>
      shouldShowSessionsLoadMore({
        ...visibleState,
        hasMoreSessions,
        hasActiveChipFilters: true,
        filteredSessionCount: 0,
      }),
    )

    expect(pageStates).toEqual([true, true, false])
  })
})

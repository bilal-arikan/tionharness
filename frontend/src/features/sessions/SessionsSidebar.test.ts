import { describe, expect, it } from 'vitest'
import { shouldShowSessionsLoadMore } from './sessionsLoadMore'

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

  it('is hidden when active chip filters leave no rendered sessions', () => {
    expect(
      shouldShowSessionsLoadMore({
        ...visibleState,
        hasActiveChipFilters: true,
      }),
    ).toBe(false)
  })

  it('is hidden when active chip filters leave 100 or fewer rendered sessions', () => {
    expect(
      shouldShowSessionsLoadMore({
        ...visibleState,
        hasActiveChipFilters: true,
        filteredSessionCount: 100,
      }),
    ).toBe(false)
  })

  it('stays visible when active chip filters leave more than 100 rendered sessions', () => {
    expect(
      shouldShowSessionsLoadMore({
        ...visibleState,
        hasActiveChipFilters: true,
        filteredSessionCount: 101,
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
})

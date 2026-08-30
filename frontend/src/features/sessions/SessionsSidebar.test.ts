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

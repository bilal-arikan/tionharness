import { describe, expect, it } from 'vitest'
import { shouldShowSessionsLoadMore } from './sessionsLoadMore'

const visibleState = {
  loading: false,
  hasMoreSessions: true,
  canLoadMore: true,
  query: '',
}

describe('shouldShowSessionsLoadMore', () => {
  it('stays visible when chip filters leave no rendered session groups', () => {
    expect(shouldShowSessionsLoadMore(visibleState)).toBe(true)
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

import { describe, expect, it } from 'vitest'
import { sessionListModel } from './sessionListModel'

describe('sessionListModel', () => {
  it('shows the current request model while a turn is running', () => {
    expect(sessionListModel('opus', 'gpt-5.6-sol', true)).toBe('gpt-5.6-sol')
  })

  it('shows the provider-confirmed session model after the turn', () => {
    expect(sessionListModel('gpt-5.6', 'gpt-5.6-sol', false)).toBe('gpt-5.6')
  })

  it('falls back to the session snapshot when the owner is unavailable', () => {
    expect(sessionListModel('claude-opus-5', undefined, true)).toBe('claude-opus-5')
  })
})

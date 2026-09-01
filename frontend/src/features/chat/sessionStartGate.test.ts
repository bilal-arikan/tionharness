import { describe, expect, it } from 'vitest'
import { shouldShowStartPanel, type StartPanelGate } from './sessionStartGate'

const base: StartPanelGate = {
  readOnly: false,
  activeSessionId: 'SES1',
  dismissedSessionId: null,
  messageCount: 0,
  messagesLoading: false,
  streaming: false,
  pending: false,
  queuedCount: 0,
  isWorker: false,
}

describe('shouldShowStartPanel', () => {
  it('shows on a fresh, empty, writable session', () => {
    expect(shouldShowStartPanel(base)).toBe(true)
  })

  it('hides the moment a turn is enqueued (send)', () => {
    expect(shouldShowStartPanel({ ...base, pending: true })).toBe(false)
    expect(shouldShowStartPanel({ ...base, queuedCount: 1 })).toBe(false)
    expect(shouldShowStartPanel({ ...base, streaming: true })).toBe(false)
  })

  it('hides once the transcript has any message', () => {
    expect(shouldShowStartPanel({ ...base, messageCount: 1 })).toBe(false)
  })

  it('waits while the transcript is still loading', () => {
    expect(shouldShowStartPanel({ ...base, messagesLoading: true })).toBe(false)
  })

  it('hides for read-only logs, workers and no session', () => {
    expect(shouldShowStartPanel({ ...base, readOnly: true })).toBe(false)
    expect(shouldShowStartPanel({ ...base, isWorker: true })).toBe(false)
    expect(shouldShowStartPanel({ ...base, activeSessionId: null })).toBe(false)
  })

  it('honours a dismissal only for the session it was made in', () => {
    expect(shouldShowStartPanel({ ...base, dismissedSessionId: 'SES1' })).toBe(false)
    expect(shouldShowStartPanel({ ...base, dismissedSessionId: 'SES2' })).toBe(true)
  })
})

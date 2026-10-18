import { describe, expect, it, vi } from 'vitest'
import type { AppEventDeps } from './useAppEvents'
import { handleAutonomousCompletion } from './useAppEvents'

vi.mock('@/api', () => ({
  api: { listMessages: vi.fn() },
  getActiveWorkspace: () => 'WS1',
}))

describe('handleAutonomousCompletion', () => {
  it('clears the pending and streaming latch when an insight scan finishes', () => {
    const clearPending = vi.fn()
    const deps = {
      chat: { clearPending },
      activeSessionId: null,
      setMessages: vi.fn(),
    } as unknown as Pick<AppEventDeps, 'chat' | 'activeSessionId' | 'setMessages'>

    handleAutonomousCompletion(deps, {
      type: 'insight',
      level: 'success',
      workspaceId: 'WS1',
      title: 'Insight scan finished',
      body: '',
      target: { sessionId: 'SES1', scanning: 'false' },
      time: 1,
    })

    expect(clearPending).toHaveBeenCalledOnce()
    expect(clearPending).toHaveBeenCalledWith('SES1')
  })
})

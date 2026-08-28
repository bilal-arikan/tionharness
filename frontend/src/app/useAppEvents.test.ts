import { describe, expect, it, vi } from 'vitest'
import type { AppEventDeps } from './useAppEvents'
import { handleAutonomousCompletion, workerBusKeys } from './useAppEvents'
import type { AppEvent } from '@/types'

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

describe('workerBusKeys', () => {
  const workerEvent = (target: Record<string, string>): AppEvent => ({
    type: 'worker',
    level: 'success',
    workspaceId: 'WS1',
    title: 'worker done',
    body: '',
    target,
    time: 1,
  })

  it('notifies the root coordinator too when the worker is nested', () => {
    const keys = workerBusKeys(
      workerEvent({ sessionId: 'SES3', coordinatorId: 'SES2', rootCoordinatorId: 'SES1' }),
    )
    expect(keys).toEqual(['SES2', 'SES1'])
  })

  it('yields a single key when the coordinator is the root', () => {
    expect(workerBusKeys(workerEvent({ sessionId: 'SES2', coordinatorId: 'SES1' }))).toEqual([
      'SES1',
    ])
  })

  it('yields nothing when the event carries no coordinator', () => {
    expect(workerBusKeys(workerEvent({ sessionId: 'SES2' }))).toEqual([])
  })
})

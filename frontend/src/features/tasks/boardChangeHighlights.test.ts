import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AppEvent } from '@/types'
import { consumePendingBoardChanges, recordPendingBoardChange } from './boardChangeHighlights'

vi.hoisted(() => {
  const values = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      clear: () => values.clear(),
      getItem: (key: string) => values.get(key) ?? null,
      removeItem: (key: string) => void values.delete(key),
      setItem: (key: string, value: string) => void values.set(key, value),
    },
  })
})

function boardEvent(workspaceId: string, taskId: string, op: string, time = 1): AppEvent {
  return {
    type: 'board',
    level: 'info',
    workspaceId,
    title: '',
    body: '',
    target: { taskId, op },
    time,
  }
}

describe('board change highlights', () => {
  beforeEach(() => localStorage.clear())

  it('consumes pending ids once', () => {
    recordPendingBoardChange(boardEvent('WS1', 'TSK1', 'update'), null)

    expect(consumePendingBoardChanges('WS1')).toEqual(new Set(['TSK1']))
    expect(consumePendingBoardChanges('WS1')).toEqual(new Set())
  })

  it('isolates pending ids by workspace', () => {
    recordPendingBoardChange(boardEvent('WS1', 'TSK1', 'create'), null)
    recordPendingBoardChange(boardEvent('WS2', 'TSK2', 'move'), null)

    expect(consumePendingBoardChanges('WS1')).toEqual(new Set(['TSK1']))
    expect(consumePendingBoardChanges('WS2')).toEqual(new Set(['TSK2']))
  })

  it('keeps distinct changes that happen in the same second', () => {
    recordPendingBoardChange(boardEvent('WS1', 'TSK1', 'update', 10), null)
    recordPendingBoardChange(boardEvent('WS1', 'TSK2', 'update', 10), null)

    expect(consumePendingBoardChanges('WS1')).toEqual(new Set(['TSK1', 'TSK2']))
  })

  it.each(['delete', 'columns_changed'])('excludes %s events', (op) => {
    recordPendingBoardChange(boardEvent('WS1', 'TSK1', op), null)
    expect(consumePendingBoardChanges('WS1')).toEqual(new Set())
  })

  it('records another workspace change while the active workspace board is open', () => {
    recordPendingBoardChange(boardEvent('WS2', 'TSK2', 'update'), 'WS1')

    expect(consumePendingBoardChanges('WS2')).toEqual(new Set(['TSK2']))
  })

  it('does not record the active workspace change while its board is open', () => {
    recordPendingBoardChange(boardEvent('WS1', 'TSK1', 'update'), 'WS1')
    expect(consumePendingBoardChanges('WS1')).toEqual(new Set())
  })
})

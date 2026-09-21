import { describe, expect, it } from 'vitest'
import { applyDisconnect, clearRecovered, dismissDisconnect } from './mcpDisconnects'
import type { MCPStatusData } from '@/api/workspaceEvents'

function ev(over: Partial<MCPStatusData> = {}): MCPStatusData {
  return {
    op: 'disconnected',
    server: 'gw',
    scoped: false,
    pendingCalls: 0,
    at: 1,
    ...over,
  }
}

describe('applyDisconnect', () => {
  it('adds a notice carrying the reason and stranded-call count', () => {
    const got = applyDisconnect([], ev({ error: 'EOF', pendingCalls: 2 }))
    expect(got).toHaveLength(1)
    expect(got[0]).toMatchObject({ server: 'gw', error: 'EOF', pendingCalls: 2 })
  })

  it('keeps one entry per server so a crash loop is one card, not a pile', () => {
    const first = applyDisconnect([], ev({ at: 1, error: 'first' }))
    const second = applyDisconnect(first, ev({ at: 2, error: 'second' }))
    expect(second).toHaveLength(1)
    expect(second[0].error).toBe('second')
  })

  it('orders newest first', () => {
    const withA = applyDisconnect([], ev({ server: 'a', at: 1 }))
    const withB = applyDisconnect(withA, ev({ server: 'b', at: 2 }))
    expect(withB.map((n) => n.server)).toEqual(['b', 'a'])
  })

  it('carries the session for a scoped loss, so the UI can scope the wording', () => {
    const got = applyDisconnect([], ev({ scoped: true, sessionId: 'sessA' }))
    expect(got[0]).toMatchObject({ scoped: true, sessionId: 'sessA' })
  })

  it('ignores an event with no server name', () => {
    expect(applyDisconnect([], ev({ server: '' }))).toHaveLength(0)
  })

  it('ignores a non-disconnect op, so a future "reconnected" cannot raise an alarm', () => {
    const other = { ...ev(), op: 'reconnected' } as unknown as MCPStatusData
    expect(applyDisconnect([], other)).toHaveLength(0)
  })
})

describe('dismissDisconnect', () => {
  it('removes only the named server', () => {
    const list = applyDisconnect(applyDisconnect([], ev({ server: 'a' })), ev({ server: 'b' }))
    expect(dismissDisconnect(list, 'a').map((n) => n.server)).toEqual(['b'])
  })
})

describe('clearRecovered', () => {
  it('drops the notice once the pool reports a live connection again', () => {
    const list = applyDisconnect([], ev({ server: 'gw' }))
    expect(clearRecovered(list, [{ server: 'gw', live: 1 }])).toHaveLength(0)
  })

  it('keeps the notice while the server has no live connection', () => {
    const list = applyDisconnect([], ev({ server: 'gw' }))
    expect(clearRecovered(list, [{ server: 'gw', live: 0 }])).toHaveLength(1)
  })

  it('keeps a notice for a server absent from the snapshot', () => {
    const list = applyDisconnect([], ev({ server: 'gw' }))
    expect(clearRecovered(list, [{ server: 'other', live: 3 }])).toHaveLength(1)
  })
})

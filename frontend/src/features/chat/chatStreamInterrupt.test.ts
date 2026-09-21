import { beforeEach, describe, expect, it, vi } from 'vitest'
import { performInterrupt } from './chatStreamInterrupt'
import type { SendContext } from './chatStreamSend'
import type { PendingItem } from './PendingTray'
import type { Session } from '@/types'

vi.mock('@/api', () => ({
  api: {
    interruptSession: vi.fn(async () => ({ result: 'ok', queued: true, stopped: true })),
    sessionControl: vi.fn(async () => ({ result: 'ok' })),
    enqueueMessage: vi.fn(async () => ({})),
  },
}))

const { api } = await import('@/api')

const SID = 'SES1'

function makeCtx(): { ctx: SendContext; queued: () => PendingItem[]; error: () => string | null } {
  let queued: PendingItem[] = []
  let error: string | null = null
  const ctx: SendContext = {
    activeSessionId: SID,
    sessions: [{ id: SID, agentId: 'AGT7' } as Session],
    thinkingLevel: 'high',
    permissionMode: 'ask',
    setPendingSessions: () => {},
    setQueued: (u) => {
      queued = typeof u === 'function' ? (u as (p: PendingItem[]) => PendingItem[])(queued) : u
    },
    setWakeWaits: () => {},
    setError: (m) => {
      error = m
    },
  }
  return { ctx, queued: () => queued, error: () => error }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('performInterrupt', () => {
  // The regression guard for the race this replaced: as two calls
  // (sessionControl('stop') then a separate send) the session's turn slot was free
  // in between, so another queued message or an autonomous turn could take it.
  it('sends ONE atomic request instead of stop-then-send', async () => {
    const { ctx } = makeCtx()

    const ok = await performInterrupt(ctx, 'kes ve bunu yap', SID, [])

    expect(ok).toBe(true)
    expect(api.interruptSession).toHaveBeenCalledTimes(1)
    expect(api.sessionControl).not.toHaveBeenCalled()
    expect(api.enqueueMessage).not.toHaveBeenCalled()
  })

  // The interrupt message becomes a normal turn, so it must carry the composer's
  // current per-turn settings; dropping them silently downgraded the turn.
  it('carries the turn settings and the session agent', async () => {
    const { ctx } = makeCtx()

    await performInterrupt(ctx, 'kes', SID, [])

    const [sid, body] = vi.mocked(api.interruptSession).mock.calls[0]
    expect(sid).toBe(SID)
    expect(body.text).toBe('kes')
    expect(body.thinkingLevel).toBe('high')
    expect(body.permissionMode).toBe('ask')
    expect(body.agentIds).toEqual(['AGT7'])
    expect(body.clientMsgId).toBeTruthy()
  })

  it('shows the message in the tray immediately', async () => {
    const { ctx, queued } = makeCtx()

    await performInterrupt(ctx, 'kes', SID, [])

    expect(queued()).toHaveLength(1)
    expect(queued()[0].text).toBe('kes')
  })

  // A failed interrupt must not eat the draft: the optimistic chip is rolled back
  // and false tells the composer to keep the text for a retry.
  it('rolls the chip back and reports failure when the request fails', async () => {
    const { ctx, queued, error } = makeCtx()
    vi.mocked(api.interruptSession).mockRejectedValueOnce(new Error('boom'))

    const ok = await performInterrupt(ctx, 'kes', SID, [])

    expect(ok).toBe(false)
    expect(queued()).toHaveLength(0)
    expect(error()).toBe('boom')
  })
})

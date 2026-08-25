import { describe, expect, it } from 'vitest'
import { pickInitialSession } from './pickInitialSession'
import type { Session } from '@/types'

function session(overrides: Partial<Session>): Session {
  return {
    id: 'SES1',
    kind: 'chat',
    agentId: 'AGT1',
    title: '',
    createdAt: 0,
    updatedAt: 0,
    ...overrides,
  } as Session
}

const noDrafts = new Set<string>()
const agentExists = () => true

describe('pickInitialSession', () => {
  it('falls back to the most recent writable session when there is no draft or deep link', () => {
    const sessions = [session({ id: 'SES2', agentId: 'AGT2' }), session({ id: 'SES1' })]
    expect(
      pickInitialSession({ sessions, wantRoute: null, draftedSessionIds: noDrafts, agentExists }),
    ).toEqual({
      sessionId: 'SES2',
      agentId: 'AGT2',
    })
  })

  it('prefers a session holding an unsent draft over the most recent one', () => {
    const sessions = [
      session({ id: 'SES2', agentId: 'AGT2' }),
      session({ id: 'SES1', agentId: 'AGT1' }),
    ]
    const drafted = new Set(['SES1'])
    expect(
      pickInitialSession({ sessions, wantRoute: null, draftedSessionIds: drafted, agentExists }),
    ).toEqual({ sessionId: 'SES1', agentId: 'AGT1' })
  })

  it('ignores a draft on a non-writable (read-only) session', () => {
    const sessions = [
      session({ id: 'SES2', agentId: 'AGT2' }),
      session({ id: 'SES1', kind: 'flow', agentId: 'AGT1' }),
    ]
    const drafted = new Set(['SES1'])
    expect(
      pickInitialSession({ sessions, wantRoute: null, draftedSessionIds: drafted, agentExists }),
    ).toEqual({ sessionId: 'SES2', agentId: 'AGT2' })
  })

  it('lets an explicit chat deep link override a drafted session', () => {
    const sessions = [
      session({ id: 'SES2', agentId: 'AGT2' }),
      session({ id: 'SES1', agentId: 'AGT1' }),
    ]
    const drafted = new Set(['SES1'])
    expect(
      pickInitialSession({
        sessions,
        wantRoute: { view: 'chat', id: 'SES2', workspaceId: null },
        draftedSessionIds: drafted,
        agentExists,
      }),
    ).toEqual({ sessionId: 'SES2', agentId: 'AGT2' })
  })

  it('honors an agents-view deep link without touching the session pick', () => {
    const sessions = [session({ id: 'SES1', agentId: 'AGT1' })]
    expect(
      pickInitialSession({
        sessions,
        wantRoute: { view: 'agents', id: 'AGT9', workspaceId: null },
        draftedSessionIds: noDrafts,
        agentExists: (id) => id === 'AGT9',
      }),
    ).toEqual({ sessionId: 'SES1', agentId: 'AGT9' })
  })
})

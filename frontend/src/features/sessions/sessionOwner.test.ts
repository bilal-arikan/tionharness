import { describe, it, expect } from 'vitest'
import type { Agent, Session } from '@/types'
import { resolveSessionOwner, sessionRowLabel } from './sessionOwner'

const agents = [{ id: 'AGT1', name: 'Kâşif' }] as Agent[]

function session(over: Partial<Session>): Session {
  return { id: 'SES1', agentId: '', kind: 'chat', title: '', ...over } as Session
}

describe('resolveSessionOwner', () => {
  it('resolves a plain session from its own agent', () => {
    const owner = resolveSessionOwner(agents, session({ agentId: 'AGT1' }))
    expect(owner?.name).toBe('Kâşif')
    expect(owner?.missing).toBe(false)
  })

  // The regression: a delegated run carries its identity as a target, so reading
  // agentId alone left the row with no avatar and no name.
  it('resolves a delegated run from its target agent', () => {
    const owner = resolveSessionOwner(agents, session({ kind: 'subagent', targetAgentId: 'AGT1' }))
    expect(owner?.name).toBe('Kâşif')
    expect(owner?.missing).toBe(false)
  })

  it('names a profile subagent, which has no agent row at all', () => {
    const owner = resolveSessionOwner(agents, session({ kind: 'subagent', targetProfile: 'coder' }))
    expect(owner?.name).toBe('subagent:coder')
    expect(owner?.deleted).toBe(false)
    expect(owner?.missing).toBe(false)
  })

  it('prefers the session agent over the target when both are set', () => {
    const owner = resolveSessionOwner(
      agents,
      session({ agentId: 'AGT1', targetAgentId: 'AGT9', targetProfile: 'coder' }),
    )
    expect(owner?.name).toBe('Kâşif')
  })

  it('still badges a deleted target rather than dropping the row', () => {
    const owner = resolveSessionOwner(agents, session({ kind: 'subagent', targetAgentId: 'AGT9' }))
    expect(owner?.missing).toBe(true)
  })

  it('returns null when the session names nobody', () => {
    expect(resolveSessionOwner(agents, session({}))).toBeNull()
  })
})

describe('sessionRowLabel', () => {
  it('uses the session title when it has one', () => {
    expect(sessionRowLabel(agents, session({ title: 'Graf taraması' }))).toBe('Graf taraması')
  })

  it('keeps the new-chat placeholder for an untitled chat', () => {
    expect(sessionRowLabel(agents, session({}))).toBe('Yeni sohbet')
  })

  // Delegated rows written before they carried a title read as empty chats
  // otherwise, which is what made every one of them show "Yeni sohbet".
  it('names an untitled delegated run after the agent that ran it', () => {
    const row = session({ kind: 'subagent', targetAgentId: 'AGT1' })
    expect(sessionRowLabel(agents, row)).toBe('Kâşif')
  })

  it('names an untitled profile subagent after its profile', () => {
    const row = session({ kind: 'subagent', targetProfile: 'coder' })
    expect(sessionRowLabel(agents, row)).toBe('subagent:coder')
  })
})

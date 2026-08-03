import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { DELETED_AGENT_LABEL, agentName, resolveAgent } from './agentLookup'

const agent = (over: Partial<Agent> & Pick<Agent, 'id' | 'name'>): Agent =>
  ({
    soul: '',
    identity: '',
    provider: 'anthropic',
    model: 'claude-opus-5',
    mcpEnabled: true,
    allowedTools: '',
    blockedTools: '',
    skills: [],
    createdAt: 0,
    updatedAt: 0,
    ...over,
  }) as Agent

const ROSTER = [
  agent({ id: 'AGT1', name: 'Canli', avatar: '🤖', color: '#fff' }),
  agent({ id: 'AGT2', name: 'Eski', deleted: true }),
]

describe('resolveAgent', () => {
  it('returns null for an absent id — nothing to render', () => {
    expect(resolveAgent(ROSTER, undefined)).toBeNull()
    expect(resolveAgent(ROSTER, '')).toBeNull()
  })

  it('resolves a live agent with its identity intact', () => {
    const got = resolveAgent(ROSTER, 'AGT1')
    expect(got).toMatchObject({ name: 'Canli', avatar: '🤖', deleted: false, missing: false })
  })

  it('keeps a deleted agent NAME and avatar so history still reads correctly', () => {
    const got = resolveAgent(ROSTER, 'AGT2')
    expect(got?.name).toBe('Eski') // the real name, not the placeholder
    expect(got?.deleted).toBe(true)
    expect(got?.missing).toBe(false)
  })

  it('never leaks a raw id for an unresolvable agent', () => {
    const got = resolveAgent(ROSTER, 'AGT-gone')
    expect(got?.name).toBe(DELETED_AGENT_LABEL)
    expect(got?.name).not.toContain('AGT-gone')
    expect(got).toMatchObject({ deleted: true, missing: true })
  })

  it('never substitutes a different agent', () => {
    // The old ChatEmptyState fallback picked agents[0] when the id did not
    // resolve, silently attributing history to the wrong agent.
    expect(resolveAgent(ROSTER, 'AGT-gone')?.id).toBe('AGT-gone')
    expect(resolveAgent(ROSTER, 'AGT-gone')?.name).not.toBe('Canli')
  })

  it('agentName is empty only when there is no id at all', () => {
    expect(agentName(ROSTER, undefined)).toBe('')
    expect(agentName(ROSTER, 'AGT1')).toBe('Canli')
    expect(agentName(ROSTER, 'yok')).toBe(DELETED_AGENT_LABEL)
  })
})

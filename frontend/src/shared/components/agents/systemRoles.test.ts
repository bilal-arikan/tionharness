import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { builtinRoleAgent, roleCandidates } from './systemRoles'

const agent = (over: Partial<Agent>): Agent => ({ id: 'x', name: 'x', ...over }) as Agent

describe('roleCandidates', () => {
  const agents = [
    agent({ id: 'mine', name: 'Mine' }),
    agent({ id: 'titler', system: true, systemKey: 'titler', locked: true }),
    agent({ id: 'compactor', system: true, systemKey: 'compaction', locked: true }),
    agent({ id: 'off', disabled: true }),
    agent({ id: 'gone', deleted: true }),
  ]

  it('offers regular agents and the role own system agent only', () => {
    expect(roleCandidates(agents, 'compaction').map((a) => a.id)).toEqual(['mine', 'compactor'])
  })

  it('keeps the saved value visible even when it is no longer eligible', () => {
    expect(roleCandidates(agents, 'compaction', 'off').map((a) => a.id)).toContain('off')
  })
})

describe('builtinRoleAgent', () => {
  it('prefers an enabled customisation over the locked built-in', () => {
    const agents = [
      agent({ id: 'b', systemKey: 'titler', locked: true }),
      agent({ id: 'c', systemKey: 'titler' }),
    ]
    expect(builtinRoleAgent(agents, 'titler')?.id).toBe('c')
    expect(builtinRoleAgent([agents[0]], 'titler')?.id).toBe('b')
  })
})

import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { groupSystemAgents, isBuiltinSystemAgent, isWorkerSystemAgent } from './agentRoster'

const agent = (over: Partial<Agent> & { id: string }): Agent =>
  ({ name: over.id, ...over }) as Agent

describe('groupSystemAgents', () => {
  it('leaves the locked built-ins out of the roster', () => {
    const groups = groupSystemAgents([
      agent({ id: 'plain' }),
      agent({ id: 'titler', system: true, locked: true, systemKey: 'titler' }),
      agent({ id: 'coder', system: true, locked: true, systemKey: 'subagent-coder' }),
    ])
    expect(groups.services).toEqual([])
    expect(groups.workers).toEqual([])
  })

  it('splits inherited customisations into services and workers in built-in order', () => {
    const groups = groupSystemAgents([
      agent({ id: 'titler', system: true, locked: true, systemKey: 'titler' }),
      agent({ id: 'compactor', system: true, locked: true, systemKey: 'compactor' }),
      agent({ id: 'coder', system: true, locked: true, systemKey: 'subagent-coder' }),
      agent({ id: 'my-compactor', system: true, locked: false, systemKey: 'compactor' }),
      agent({ id: 'my-coder', system: true, locked: false, systemKey: 'subagent-coder' }),
      agent({ id: 'my-titler', system: true, locked: false, systemKey: 'titler' }),
    ])
    expect(groups.services.map((a) => a.id)).toEqual(['my-titler', 'my-compactor'])
    expect(groups.workers.map((a) => a.id)).toEqual(['my-coder'])
  })

  it('still lists a customisation whose built-in is not seeded', () => {
    const groups = groupSystemAgents([
      agent({ id: 'orphan', system: true, locked: false, systemKey: 'subagent-planner' }),
    ])
    expect(groups.services).toEqual([])
    expect(groups.workers.map((a) => a.id)).toEqual(['orphan'])
  })

  it('flags only locked system agents as built-ins', () => {
    expect(isBuiltinSystemAgent(agent({ id: 'b', system: true, locked: true }))).toBe(true)
    expect(isBuiltinSystemAgent(agent({ id: 'c', system: true, locked: false }))).toBe(false)
    expect(isBuiltinSystemAgent(agent({ id: 'p' }))).toBe(false)
  })

  it('treats a missing system key as a service row', () => {
    expect(isWorkerSystemAgent(agent({ id: 'x', system: true }))).toBe(false)
  })
})

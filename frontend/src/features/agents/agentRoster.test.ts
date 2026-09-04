import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { groupSystemAgents, isWorkerSystemAgent } from './agentRoster'

const agent = (over: Partial<Agent> & { id: string }): Agent =>
  ({ name: over.id, ...over }) as Agent

describe('groupSystemAgents', () => {
  it('splits worker profiles out of the service built-ins', () => {
    const groups = groupSystemAgents([
      agent({ id: 'plain' }),
      agent({ id: 'titler', system: true, locked: true, systemKey: 'titler' }),
      agent({ id: 'coder', system: true, locked: true, systemKey: 'subagent-coder' }),
    ])
    expect(groups.services.map((a) => a.id)).toEqual(['titler'])
    expect(groups.workers.map((a) => a.id)).toEqual(['coder'])
  })

  it('keeps a customisation next to the built-in it is bound to', () => {
    const groups = groupSystemAgents([
      agent({ id: 'titler', system: true, locked: true, systemKey: 'titler' }),
      agent({ id: 'coder', system: true, locked: true, systemKey: 'subagent-coder' }),
      agent({ id: 'my-coder', system: true, locked: false, systemKey: 'subagent-coder' }),
      agent({ id: 'my-titler', system: true, locked: false, systemKey: 'titler' }),
    ])
    expect(groups.services.map((a) => a.id)).toEqual(['titler', 'my-titler'])
    expect(groups.workers.map((a) => a.id)).toEqual(['coder', 'my-coder'])
  })

  it('still lists a customisation whose built-in is not seeded', () => {
    const groups = groupSystemAgents([
      agent({ id: 'orphan', system: true, locked: false, systemKey: 'subagent-planner' }),
    ])
    expect(groups.services).toEqual([])
    expect(groups.workers.map((a) => a.id)).toEqual(['orphan'])
  })

  it('treats a missing system key as a service row', () => {
    expect(isWorkerSystemAgent(agent({ id: 'x', system: true }))).toBe(false)
  })
})

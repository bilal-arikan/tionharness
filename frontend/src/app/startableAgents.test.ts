import { describe, it, expect } from 'vitest'
import type { Agent } from '@/types'
import { isStartableAgent, startableAgents } from './startableAgents'

function agent(over: Partial<Agent>): Agent {
  return { id: 'a', name: 'A', ...over } as Agent
}

describe('isStartableAgent', () => {
  it('accepts a plain user agent', () => {
    expect(isStartableAgent(agent({ id: 'dev', name: 'Developer' }))).toBe(true)
  })

  it('rejects a service system agent', () => {
    expect(isStartableAgent(agent({ system: true, systemKey: 'titler' }))).toBe(false)
    expect(isStartableAgent(agent({ system: true, systemKey: 'compaction' }))).toBe(false)
    expect(isStartableAgent(agent({ system: true, systemKey: 'insight' }))).toBe(false)
  })

  it('rejects an archived agent (it cannot run until restored)', () => {
    expect(isStartableAgent(agent({ id: 'dev', archived: true }))).toBe(false)
  })

  it('accepts a worker profile even though it is a system agent', () => {
    expect(isStartableAgent(agent({ system: true, systemKey: 'subagent-coder' }))).toBe(true)
    expect(isStartableAgent(agent({ system: true, systemKey: 'subagent-explore' }))).toBe(true)
  })

  it('accepts a workspace customisation bound to a worker role', () => {
    const custom = agent({ system: true, locked: false, systemKey: 'subagent-config' })
    expect(isStartableAgent(custom)).toBe(true)
  })

  it('rejects a customisation bound to a service role', () => {
    const custom = agent({ system: true, locked: false, systemKey: 'overview-summarizer' })
    expect(isStartableAgent(custom)).toBe(false)
  })
})

describe('startableAgents', () => {
  it('keeps user agents and workers, drops service agents, preserving order', () => {
    const roster = [
      agent({ id: 'AGT101', system: true, systemKey: 'subagent-explore' }),
      agent({ id: 'AGT198' }),
      agent({ id: 'AGT200', system: true, systemKey: 'titler' }),
      agent({ id: 'AGT232', system: true, systemKey: 'compaction' }),
      agent({ id: 'AGT266' }),
    ]
    expect(startableAgents(roster).map((a) => a.id)).toEqual(['AGT101', 'AGT198', 'AGT266'])
  })

  it('returns an empty list rather than throwing when every agent is a service agent', () => {
    const roster = [agent({ id: 'AGT200', system: true, systemKey: 'titler' })]
    expect(startableAgents(roster)).toEqual([])
  })
})

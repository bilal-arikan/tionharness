import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import { augmentLive, isLiveAgentRef, liveAgentRef, panelRefFor } from './explorerLive'

const ROOT: ViewRef = { kind: 'workspace', id: 'workspace' }
const sessions: ViewRef = { kind: 'category', id: 'sessions' }
const s1: ViewRef = { kind: 'session', id: 'SES1' }
const s2: ViewRef = { kind: 'session', id: 'SES2' }
const agent = { id: 'AG1', name: 'builder', emoji: '🔧', color: '#123456' }

const graph: ViewGraphResult = {
  nodes: [
    { label: 'workspace', ref: ROOT },
    { label: 'Oturumlar', ref: sessions },
    { label: 'session:SES1 a', ref: s1 },
    { label: 'session:SES2 b', ref: s2 },
  ],
  edges: [
    { source: ROOT, target: sessions },
    { source: sessions, target: s1 },
    { source: sessions, target: s2 },
  ],
  live: [
    { session: s1, state: 'running', agent },
    { session: s2, state: 'awaiting-workers', agent },
    { session: { kind: 'session', id: 'GONE' }, state: 'running', agent },
  ],
}

describe('explorerLive', () => {
  it('derives one avatar node per live session, tethered to it, and skips sessions off the map', () => {
    const { graph: out, liveState, liveAgents } = augmentLive(graph)
    expect(out.nodes).toHaveLength(6)
    expect(out.edges).toHaveLength(5)
    const k1 = 'agent:AG1#live:SES1'
    const k2 = 'agent:AG1#live:SES2'
    expect(liveAgents.get(k1)?.state).toBe('running')
    expect(liveAgents.get(k2)?.state).toBe('awaiting-workers')
    expect(liveState.get('session:SES1')).toBe('running')
    expect(liveState.has('session:GONE')).toBe(false)
    expect(out.edges.at(-2)).toEqual({ source: s1, target: liveAgentRef(graph.live![0]) })
    expect(out.nodes.at(-1)?.label).toBe('builder')
    // The input is not mutated.
    expect(graph.nodes).toHaveLength(4)
  })

  it('an avatar node projects its agent, everything else projects itself', () => {
    const live = liveAgentRef(graph.live![0])
    expect(isLiveAgentRef(live)).toBe(true)
    expect(isLiveAgentRef({ kind: 'agent', id: 'AG1' })).toBe(false)
    expect(panelRefFor(live)).toEqual({ kind: 'agent', id: 'AG1' })
    expect(panelRefFor(s1)).toBe(s1)
  })

  it('a payload without a live layer yields no avatars', () => {
    const { graph: out, liveAgents } = augmentLive({ nodes: graph.nodes, edges: graph.edges })
    expect(out.nodes).toHaveLength(4)
    expect(liveAgents.size).toBe(0)
  })
})

import { describe, expect, it } from 'vitest'
import type { Agent } from '@/types'
import { eligibleParents, indexAgents, isSelfOrDescendant, lineageOf } from './agentLineage'

const mk = (id: string, extra: Partial<Agent> = {}) =>
  ({ id, name: id, provider: 'claude-cli', model: '', ...extra }) as Agent

const root = mk('root', { locked: true, system: true, systemKey: 'titler' })
const mid = mk('mid', { parentId: 'root' })
const leaf = mk('leaf', { parentId: 'mid' })
const other = mk('other')
const bound = mk('bound', { parentId: 'root', system: true, systemKey: 'titler' })
const all = [root, mid, leaf, other, bound]

describe('agentLineage', () => {
  it('lists ancestors root first', () => {
    expect(lineageOf(leaf, indexAgents(all)).map((a) => a.id)).toEqual(['root', 'mid'])
    expect(lineageOf(root, indexAgents(all))).toEqual([])
  })

  it('stops at an unknown parent and never loops on a cycle', () => {
    const orphan = mk('orphan', { parentId: 'missing' })
    expect(lineageOf(orphan, indexAgents([orphan]))).toEqual([])
    const a = mk('a', { parentId: 'b' })
    const b = mk('b', { parentId: 'a' })
    expect(lineageOf(a, indexAgents([a, b])).map((x) => x.id)).toEqual(['b'])
  })

  it('excludes self and descendants from parent candidates', () => {
    expect(isSelfOrDescendant(mid, 'leaf', all)).toBe(true)
    expect(isSelfOrDescendant(mid, 'mid', all)).toBe(true)
    expect(isSelfOrDescendant(mid, 'root', all)).toBe(false)
    expect(eligibleParents(mid, all).map((a) => a.id)).toEqual(['root', 'other', 'bound'])
  })

  it('gives a bound system customisation no parent choice', () => {
    expect(eligibleParents(bound, all)).toEqual([])
  })
})

// Agent inheritance helpers shared by the roster, the settings form and the
// pickers. The server already RESOLVES every agent (parent values folded in), so
// the client only needs the tree shape: who inherits from whom.
import type { Agent, AgentOverrideKey } from '@/types'

// Bounds a walk over a hand-edited store that somehow contains a cycle; the
// backend rejects cycles on write, so this is defensive only.
const MAX_DEPTH = 32

export type AgentIndex = Map<string, Agent>

export function indexAgents(agents: Agent[]): AgentIndex {
  return new Map(agents.map((a) => [a.id, a]))
}

// Ancestors of `agent`, ROOT FIRST and the direct parent last. Empty for a
// root agent or when the parent is unknown to the index.
export function lineageOf(agent: Pick<Agent, 'id' | 'parentId'>, byId: AgentIndex): Agent[] {
  const chain: Agent[] = []
  const seen = new Set<string>([agent.id])
  let cursor = agent.parentId
  while (cursor && chain.length < MAX_DEPTH) {
    const parent = byId.get(cursor)
    if (!parent || seen.has(parent.id)) break
    seen.add(parent.id)
    chain.unshift(parent)
    cursor = parent.parentId
  }
  return chain
}

// Live direct children of `id`, in roster order.
function childrenOf(id: string, agents: Agent[]): Agent[] {
  return agents.filter((a) => a.parentId === id && !a.deleted)
}

// Whether `candidateId` is `agent` itself or one of its descendants — the set
// an agent may NOT pick as its parent.
export function isSelfOrDescendant(agent: Agent, candidateId: string, agents: Agent[]): boolean {
  if (agent.id === candidateId) return true
  const stack = [agent.id]
  const seen = new Set<string>()
  while (stack.length) {
    const cur = stack.pop()!
    if (seen.has(cur)) continue
    seen.add(cur)
    for (const child of childrenOf(cur, agents)) {
      if (child.id === candidateId) return true
      stack.push(child.id)
    }
  }
  return false
}

// Agents `agent` may inherit from: live, not itself, not a descendant. A bound
// system customisation keeps its built-in parent, so it gets no choice here.
export function eligibleParents(agent: Agent, agents: Agent[]): Agent[] {
  if (agent.system && !agent.locked) return []
  return agents.filter((a) => !a.deleted && !isSelfOrDescendant(agent, a.id, agents))
}

export function hasOverride(agent: Pick<Agent, 'overrides'>, key: AgentOverrideKey): boolean {
  return !!agent.overrides?.includes(key)
}

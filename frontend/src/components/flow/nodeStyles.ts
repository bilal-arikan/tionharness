// Shared visual styling for flow canvas nodes: per-type accent + run status.
import { createContext, useContext } from 'react'
import { useStore } from '@xyflow/react'
import type { Agent } from '../../types'
import type { NodeStatus } from '../../lib/flowGraph'

// useIsEndNode reports whether a node is terminal (has no outgoing edge), so the
// canvas can tint it like an "end" of the flow. Reads edges from the React Flow
// store so it updates live as connections change.
export function useIsEndNode(id: string): boolean {
  return useStore((s) => !s.edges.some((e) => e.source === id))
}

// Faint body tints (mixed into the surface) to distinguish flow endpoints.
export const START_TINT = 'color-mix(in srgb, #22c55e 14%, var(--color-surface))'
export const END_TINT = 'color-mix(in srgb, #3b82f6 14%, var(--color-surface))'

// AgentsContext lets node components resolve an agentId to its display info
// without threading agents through every node's data.
export const AgentsContext = createContext<Agent[]>([])

export function useAgent(agentId?: string): Agent | undefined {
  const agents = useContext(AgentsContext)
  return agents.find((a) => a.id === agentId)
}

export interface NodeChrome {
  accent: string // border/handle color
  label: string
  icon: string
}

const CHROME: Record<string, NodeChrome> = {
  agent: { accent: 'var(--color-accent)', label: 'Ajan', icon: '🤖' },
  branch: { accent: '#d97706', label: 'Dallanma', icon: '🔀' },
  parallel: { accent: '#7c3aed', label: 'Paralel', icon: '⚡' },
}

export function chromeFor(type: string): NodeChrome {
  return CHROME[type] ?? { accent: 'var(--color-border)', label: type, icon: '•' }
}

// statusRing returns an extra box-shadow style for a node's run status.
export function statusRing(status?: NodeStatus): string {
  switch (status) {
    case 'running':
      return '0 0 0 2px var(--color-accent), 0 0 16px var(--color-accent)'
    case 'done':
      return '0 0 0 2px #22c55e'
    case 'error':
      return '0 0 0 2px var(--color-danger)'
    default:
      return 'none'
  }
}

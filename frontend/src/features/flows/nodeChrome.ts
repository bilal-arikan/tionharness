// Per-type look of a flow node: icon, accent and the i18n label key.
import { Bot, GitBranch, Play, Puzzle, Square, Zap, type LucideIcon } from 'lucide-react'
import type { FlowNodeType } from '@/types'
import type { NodeStatus } from './flowGraph'

export interface NodeChrome {
  Icon: LucideIcon
  accent: string
}

export const NODE_CHROME: Record<FlowNodeType, NodeChrome> = {
  input: { Icon: Play, accent: 'var(--color-success)' },
  llm: { Icon: Bot, accent: 'var(--color-accent)' },
  route: { Icon: GitBranch, accent: 'var(--color-warning)' },
  transform: { Icon: Puzzle, accent: 'var(--color-info)' },
  // The automations screen's tag-lane purple, so a trigger reads as "an automation".
  trigger: { Icon: Zap, accent: '#8b5cf6' },
  output: { Icon: Square, accent: 'var(--color-success)' },
}

export const ADDABLE_TYPES: FlowNodeType[] = ['llm', 'route', 'transform', 'trigger']

export function statusRing(status?: NodeStatus): string {
  switch (status) {
    case 'running':
      return '0 0 0 2px var(--color-accent), 0 0 18px var(--color-accent)'
    case 'done':
      return '0 0 0 2px var(--color-success)'
    case 'error':
      return '0 0 0 2px var(--color-danger)'
    default:
      return 'none'
  }
}

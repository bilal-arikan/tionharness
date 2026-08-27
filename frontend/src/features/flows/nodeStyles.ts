// Shared visual styling for flow canvas nodes: per-type accent + run status.
import { createContext, useContext } from 'react'
import { useStore } from '@xyflow/react'
import {
  Bot,
  Split,
  Zap,
  Timer,
  Puzzle,
  Repeat,
  MessageCircleQuestion,
  Workflow,
  Play,
  Square,
  Circle,
  Rocket,
  GitMerge,
  Compass,
  type LucideIcon,
} from 'lucide-react'
import type { Agent, FlowNodeType } from '@/types'
import type { NodeStatus } from './flowGraph'

// useIsEndNode reports whether a node is terminal (has no outgoing edge), so the
// canvas can tint it like an "end" of the flow. Reads edges from the React Flow
// store so it updates live as connections change.
export function useIsEndNode(id: string): boolean {
  return useStore((s) => !s.edges.some((e) => e.source === id))
}

// Faint body tints (mixed into the surface) to distinguish flow endpoints.
export const START_TINT = 'color-mix(in srgb, var(--color-success) 14%, var(--color-surface))'
export const END_TINT = 'color-mix(in srgb, var(--color-info) 14%, var(--color-surface))'

// AgentsContext lets node components resolve an agentId to its display info
// without threading agents through every node's data.
export const AgentsContext = createContext<Agent[]>([])

// NodeActions are the per-node toolbar actions, provided by the editor and
// invoked from a node's NodeToolbar. Null in read-only previews.
export interface NodeActions {
  onMakeStart: (id: string) => void
  onDuplicate: (id: string) => void
  onDelete: (id: string) => void
}

export const NodeActionsContext = createContext<NodeActions | null>(null)

export function useAgent(agentId?: string): Agent | undefined {
  const agents = useContext(AgentsContext)
  return agents.find((a) => a.id === agentId)
}

export interface NodeChrome {
  accent: string // border/handle color
  label: string
  Icon: LucideIcon // monochrome (theme-colored) type glyph — inherits currentColor
}

// Monochrome lucide glyph per node type. Rendered in the accent-colored node
// header (white via currentColor) AND in the "Node ekle" palette tree (theme
// text color) — replacing the previous multicolor emojis.
export const NODE_ICONS: Record<FlowNodeType, LucideIcon> = {
  agent: Bot,
  branch: Split,
  parallel: Zap,
  delay: Timer,
  transform: Puzzle,
  loop: Repeat,
  'await-input': MessageCircleQuestion,
  subflow: Workflow,
  start: Play,
  end: Square,
  spawn: Rocket,
  join: GitMerge,
  coordinator: Compass,
}

const CHROME: Record<string, NodeChrome> = {
  agent: { accent: 'var(--color-accent)', label: 'Ajan', Icon: NODE_ICONS.agent },
  branch: { accent: '#d97706', label: 'Dallanma', Icon: NODE_ICONS.branch },
  parallel: { accent: '#7c3aed', label: 'Paralel', Icon: NODE_ICONS.parallel },
  delay: { accent: '#0891b2', label: 'Bekle', Icon: NODE_ICONS.delay },
  transform: { accent: '#059669', label: 'Birleştir', Icon: NODE_ICONS.transform },
  loop: { accent: '#db2777', label: 'Döngü', Icon: NODE_ICONS.loop },
  'await-input': { accent: '#eab308', label: 'Girdi Bekle', Icon: NODE_ICONS['await-input'] },
  subflow: { accent: '#6366f1', label: 'Alt-Akış', Icon: NODE_ICONS.subflow },
  start: { accent: '#22c55e', label: 'Başlangıç', Icon: NODE_ICONS.start },
  end: { accent: '#3b82f6', label: 'Bitiş', Icon: NODE_ICONS.end },
  spawn: { accent: '#c026d3', label: 'Spawn (Async)', Icon: NODE_ICONS.spawn },
  join: { accent: '#0d9488', label: 'Join (Bariyer)', Icon: NODE_ICONS.join },
  coordinator: { accent: '#ea580c', label: 'Koordinatör', Icon: NODE_ICONS.coordinator },
}

export function chromeFor(type: string): NodeChrome {
  return CHROME[type] ?? { accent: 'var(--color-border)', label: type, Icon: Circle }
}

// statusRing returns an extra box-shadow style for a node's run status.
export function statusRing(status?: NodeStatus): string {
  switch (status) {
    case 'running':
      return '0 0 0 2px var(--color-accent), 0 0 16px var(--color-accent)'
    case 'done':
      return '0 0 0 2px var(--color-success), 0 0 12px color-mix(in srgb, var(--color-success) 55%, transparent)'
    case 'error':
      return '0 0 0 2px var(--color-danger)'
    case 'waiting':
      return '0 0 0 2px var(--color-warning), 0 0 12px color-mix(in srgb, var(--color-warning) 55%, transparent)'
    default:
      return 'none'
  }
}

import type { LucideIcon } from 'lucide-react'
import { NODE_ICONS } from './nodeStyles'
import type { EdgeStyle } from './FlowCanvas'
import type { FlowNodeType, FlowState } from '@/types'

// Left-column tab of the flows screen: own flows, read-only template gallery,
// or run history.
export type FlowsTab = 'flows' | 'templates' | 'runs'

// Node palette entries. Icons are the shared monochrome (theme-colored) lucide
// glyphs from nodeStyles, so the palette tree matches the canvas node headers.
export const NODE_TYPES: { value: FlowNodeType; label: string; Icon: LucideIcon }[] = [
  { value: 'agent', label: 'Ajan', Icon: NODE_ICONS.agent },
  { value: 'branch', label: 'Dallanma', Icon: NODE_ICONS.branch },
  { value: 'parallel', label: 'Paralel', Icon: NODE_ICONS.parallel },
  { value: 'delay', label: 'Bekle', Icon: NODE_ICONS.delay },
  { value: 'transform', label: 'Birleştir', Icon: NODE_ICONS.transform },
  { value: 'loop', label: 'Döngü', Icon: NODE_ICONS.loop },
]

export const EDGE_STYLES: { value: EdgeStyle; label: string }[] = [
  { value: 'default', label: 'Eğri' },
  { value: 'smoothstep', label: 'Yumuşak' },
  { value: 'step', label: 'Basamak' },
  { value: 'straight', label: 'Düz' },
]

export function safeParse(s: string): FlowState | null {
  try {
    return JSON.parse(s) as FlowState
  } catch {
    return null
  }
}

// flowNodeCount reads how many nodes a flow's stored graph holds, for the list
// meta line. Best-effort — an unparseable graph reads as 0.
export function flowNodeCount(graph: string): number {
  try {
    const g = JSON.parse(graph || '{}')
    return Array.isArray(g.nodes) ? g.nodes.length : 0
  } catch {
    return 0
  }
}

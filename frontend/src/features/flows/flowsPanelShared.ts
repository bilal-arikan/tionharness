import type { LucideIcon } from 'lucide-react'
import { NODE_ICONS } from './nodeStyles'
import type { EdgeStyle } from './FlowCanvas'
import type { FlowNodeType, FlowState } from '@/types'
import { i18next } from '@/i18n'

// Left-column tab of the flows screen: own flows, read-only template gallery,
// or run history.
export type FlowsTab = 'flows' | 'templates' | 'runs'

// Node palette entries. Icons are the shared monochrome (theme-colored) lucide
// glyphs from nodeStyles, so the palette tree matches the canvas node headers.
export const NODE_TYPES: { value: FlowNodeType; label: string; Icon: LucideIcon }[] = [
  {
    value: 'start',
    get label() {
      return i18next.t('nodeTypes.start', { ns: 'flows' })
    },
    Icon: NODE_ICONS.start,
  },
  {
    value: 'end',
    get label() {
      return i18next.t('nodeTypes.end', { ns: 'flows' })
    },
    Icon: NODE_ICONS.end,
  },
  {
    value: 'agent',
    get label() {
      return i18next.t('nodeTypes.agent', { ns: 'flows' })
    },
    Icon: NODE_ICONS.agent,
  },
  {
    value: 'branch',
    get label() {
      return i18next.t('nodeTypes.branch', { ns: 'flows' })
    },
    Icon: NODE_ICONS.branch,
  },
  {
    value: 'parallel',
    get label() {
      return i18next.t('nodeTypes.parallel', { ns: 'flows' })
    },
    Icon: NODE_ICONS.parallel,
  },
  {
    value: 'delay',
    get label() {
      return i18next.t('nodeTypes.delay', { ns: 'flows' })
    },
    Icon: NODE_ICONS.delay,
  },
  {
    value: 'transform',
    get label() {
      return i18next.t('nodeTypes.transform', { ns: 'flows' })
    },
    Icon: NODE_ICONS.transform,
  },
  {
    value: 'loop',
    get label() {
      return i18next.t('nodeTypes.loop', { ns: 'flows' })
    },
    Icon: NODE_ICONS.loop,
  },
  {
    value: 'await-input',
    get label() {
      return i18next.t('nodeTypes.await-input', { ns: 'flows' })
    },
    Icon: NODE_ICONS['await-input'],
  },
  {
    value: 'subflow',
    get label() {
      return i18next.t('nodeTypes.subflow', { ns: 'flows' })
    },
    Icon: NODE_ICONS.subflow,
  },
  {
    value: 'spawn',
    get label() {
      return i18next.t('nodeTypes.spawn', { ns: 'flows' })
    },
    Icon: NODE_ICONS.spawn,
  },
  {
    value: 'join',
    get label() {
      return i18next.t('nodeTypes.join', { ns: 'flows' })
    },
    Icon: NODE_ICONS.join,
  },
  {
    value: 'coordinator',
    get label() {
      return i18next.t('nodeTypes.coordinator', { ns: 'flows' })
    },
    Icon: NODE_ICONS.coordinator,
  },
]

export const EDGE_STYLES: { value: EdgeStyle; label: string }[] = [
  {
    value: 'default',
    get label() {
      return i18next.t('edgeStyles.default', { ns: 'flows' })
    },
  },
  {
    value: 'smoothstep',
    get label() {
      return i18next.t('edgeStyles.smoothstep', { ns: 'flows' })
    },
  },
  {
    value: 'step',
    get label() {
      return i18next.t('edgeStyles.step', { ns: 'flows' })
    },
  },
  {
    value: 'straight',
    get label() {
      return i18next.t('edgeStyles.straight', { ns: 'flows' })
    },
  },
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

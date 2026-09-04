import type { Edge, Node } from 'vis-network'
import type { ViewGraphResult, ViewHandle, ViewKind, ViewRef } from '@/types'
import { refToString } from '@/types'
import type { SeedLayout } from './explorerSeed'

// Maps the whole-workspace structural map (GET /api/views/graph) onto vis-network
// nodes/edges. Same visual language as the Ağ screen (relationGraph.ts) so a
// session, flow or skill looks the same on both canvases; the workspace root and
// its buckets are the hubs the force field arranges everything around.

export const ROOT_REF: ViewRef = { kind: 'workspace', id: 'workspace' }
export const ROOT_KEY = refToString(ROOT_REF)

export interface ExplorerTheme {
  bg: string
  surface: string
  surface2: string
  border: string
  text: string
  textDim: string
  accent: string
  warning: string
}

const THEME_FALLBACK: ExplorerTheme = {
  bg: '#0f172a',
  surface: '#1e293b',
  surface2: '#273449',
  border: '#334155',
  text: '#e2e8f0',
  textDim: '#94a3b8',
  accent: '#38bdf8',
  warning: '#f59e0b',
}

// resolveExplorerTheme reads the app's CSS tokens. Falls back per token instead
// of throwing: the map must still render inside a test DOM or a mid-transition
// theme swap, and a slightly-off color is better than a blank canvas.
export function resolveExplorerTheme(): ExplorerTheme {
  const styles = getComputedStyle(document.documentElement)
  const read = (token: string, fallback: string) =>
    styles.getPropertyValue(token).trim() || fallback
  return {
    bg: read('--color-bg', THEME_FALLBACK.bg),
    surface: read('--color-surface', THEME_FALLBACK.surface),
    surface2: read('--color-surface-2', THEME_FALLBACK.surface2),
    border: read('--color-border', THEME_FALLBACK.border),
    text: read('--color-text', THEME_FALLBACK.text),
    textDim: read('--color-text-dim', THEME_FALLBACK.textDim),
    accent: read('--color-accent', THEME_FALLBACK.accent),
    warning: read('--color-warning', THEME_FALLBACK.warning),
  }
}

export const KIND_LABEL: Record<ViewKind, string> = {
  workspace: 'Workspace',
  category: 'Grup',
  board: 'Pano',
  session: 'Oturum',
  flowrun: 'Akış çalıştırması',
  schedule: 'Zamanlama',
  agent: 'Ajan',
  budget: 'Bütçe',
  tools: 'Araçlar',
  artifact: 'Artifact',
  automation: 'Otomasyon',
  skill: 'Skill',
  insight: 'İçgörü',
  logs: 'Günlükler',
  trajectory: 'Rota',
}

// Bucket colors: one hue per top-level group so the ring reads at a glance and a
// member inherits its group's hue as a border tint.
const CATEGORY_COLOR: Record<string, string> = {
  sessions: '#0ea5e9',
  flows: '#7c3aed',
  agents: '#10b981',
  artifacts: '#f59e0b',
  automations: '#f97316',
  skills: '#eab308',
  insights: '#ec4899',
}
const BOARD_COLUMN_COLOR: Record<string, string> = {
  todo: '#64748b',
  in_progress: '#0ea5e9',
  review: '#f59e0b',
  done: '#10b981',
  failed: '#ef4444',
}
const KIND_COLOR: Partial<Record<ViewKind, string>> = {
  board: '#64748b',
  logs: '#a1a1aa',
  budget: '#22c55e',
  tools: '#14b8a6',
  session: '#0ea5e9',
  flowrun: '#7c3aed',
  schedule: '#6366f1',
  agent: '#10b981',
  artifact: '#f59e0b',
  automation: '#f97316',
  skill: '#eab308',
  insight: '#ec4899',
  trajectory: '#a855f7',
}

export function kindColor(ref: ViewRef): string {
  if (ref.kind === 'workspace') return THEME_FALLBACK.accent
  if (ref.kind === 'category') {
    if (ref.id.startsWith('col:')) {
      return BOARD_COLUMN_COLOR[ref.id.slice('col:'.length)] ?? '#64748b'
    }
    return CATEGORY_COLOR[ref.id] ?? '#64748b'
  }
  return KIND_COLOR[ref.kind] ?? '#64748b'
}

// displayLabel strips the "kind:ID " spelling the backend prefixes handle labels
// with ("session:SES12 Fix login" → "Fix login"); the ref stays in the tooltip.
export function displayLabel(handle: ViewHandle): string {
  const key = refToString(handle.ref)
  let label = handle.label.trim()
  const prefixes = [key, `${handle.ref.kind}:${handle.ref.id}`, `rota:${handle.ref.id}`]
  for (const prefix of prefixes) {
    if (label.startsWith(prefix)) {
      label = label.slice(prefix.length).trim()
      break
    }
  }
  return label || handle.ref.id
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n - 1) + '…' : s
}

function esc(s: string): string {
  const d = document.createElement('div')
  d.textContent = s
  return d.innerHTML
}

function tooltip(heading: string, lines: string[]): HTMLElement {
  const el = document.createElement('div')
  el.style.maxWidth = '320px'
  el.style.whiteSpace = 'normal'
  el.style.lineHeight = '1.35'
  el.innerHTML =
    `<div style="font-weight:600;margin-bottom:3px">${esc(heading)}</div>` +
    lines.map((l) => `<div style="opacity:.75;font-size:12px">${esc(l)}</div>`).join('')
  return el
}

// Visual budget per depth: the hub is big and heavy, buckets medium, members
// light — the force field then naturally keeps the hierarchy concentric.
function depthStyle(depth: number): { size: number; mass: number; fontSize: number } {
  if (depth === 0) return { size: 38, mass: 12, fontSize: 16 }
  if (depth === 1) return { size: 22, mass: 4, fontSize: 13 }
  return { size: 12, mass: 1, fontSize: 11 }
}

function shapeFor(kind: ViewKind, depth: number): Node['shape'] {
  if (depth <= 1) return 'dot'
  switch (kind) {
    case 'session':
      return 'box'
    case 'flowrun':
    case 'trajectory':
      return 'diamond'
    case 'skill':
      return 'star'
    case 'automation':
      return 'triangle'
    case 'schedule':
      return 'triangleDown'
    case 'artifact':
      return 'square'
    case 'insight':
      return 'hexagon'
    case 'category':
      return 'dot'
    default:
      return 'dot'
  }
}

export interface ExplorerVisOptions {
  selectedKey: string | null
  search: string
  theme: ExplorerTheme
  layout: SeedLayout
}

export interface ExplorerVisData {
  nodes: Node[]
  edges: Edge[]
}

// graphToVis is pure: the same graph + options give the same nodes/edges, so the
// incremental DataSet sync in VisNetworkGraph sees stable ids and only the fields
// that changed (selection ring, search dimming).
export function graphToVis(graph: ViewGraphResult, opts: ExplorerVisOptions): ExplorerVisData {
  const needle = opts.search.trim().toLowerCase()
  const dimmed = new Set<string>()
  const nodes: Node[] = graph.nodes.map((handle) => {
    const key = refToString(handle.ref)
    const depth = opts.layout.depth[key] ?? 2
    const label = key === ROOT_KEY ? 'Workspace' : displayLabel(handle)
    const matches =
      needle === '' || label.toLowerCase().includes(needle) || key.toLowerCase().includes(needle)
    if (!matches) dimmed.add(key)
    const selected = key === opts.selectedKey
    const color = key === ROOT_KEY ? opts.theme.accent : kindColor(handle.ref)
    const { size, mass, fontSize } = depthStyle(depth)
    const shape = shapeFor(handle.ref.kind, depth)
    const boxed = shape === 'box'
    const seed = opts.layout.positions[key]
    const node: Node = {
      id: key,
      label: truncate(label, depth <= 1 ? 24 : 26),
      title: tooltip(label, [
        KIND_LABEL[handle.ref.kind],
        key,
        ...(handle.ref.kind === 'session' ? ['↗ çift tık sohbeti açar'] : []),
      ]),
      shape,
      size,
      mass,
      opacity: matches ? 1 : 0.15,
      borderWidth: selected ? 4 : depth <= 1 ? 2 : 1.5,
      color: {
        background: boxed ? opts.theme.surface2 : color,
        border: selected ? opts.theme.text : color,
        highlight: { background: boxed ? opts.theme.surface2 : color, border: opts.theme.text },
        hover: { background: boxed ? opts.theme.surface2 : color, border: opts.theme.text },
      },
      font: {
        color: opts.theme.text,
        size: fontSize,
        strokeWidth: boxed ? 0 : 3,
        strokeColor: opts.theme.bg,
        bold: depth === 0 ? { color: opts.theme.text, size: fontSize, mod: 'bold' } : undefined,
      },
      ...(boxed
        ? { shapeProperties: { borderRadius: 6 }, margin: { top: 5, bottom: 5, left: 9, right: 9 } }
        : {}),
      ...(seed ? { x: seed.x, y: seed.y } : {}),
    }
    return node
  })

  const pairs = new Set(
    graph.edges.map((edge) => `${refToString(edge.source)} ${refToString(edge.target)}`),
  )
  const edges: Edge[] = graph.edges.map((edge) => {
    const source = refToString(edge.source)
    const target = refToString(edge.target)
    const cyclic = source === target || pairs.has(`${target} ${source}`)
    const targetDepth = opts.layout.depth[target] ?? 2
    const faded = dimmed.has(source) || dimmed.has(target)
    const stroke = cyclic ? opts.theme.warning : opts.theme.border
    return {
      id: `${source}->${target}`,
      from: source,
      to: target,
      arrows: { to: { enabled: true, scaleFactor: 0.45 } },
      dashes: cyclic ? [6, 4] : false,
      length: targetDepth <= 1 ? 320 : targetDepth === 2 ? 150 : 110,
      width: targetDepth <= 1 ? 1.5 : 1,
      color: { color: stroke, highlight: opts.theme.textDim, opacity: faded ? 0.08 : 0.7 },
      title:
        source === target
          ? `${source} kendi üzerine döngü`
          : cyclic
            ? `${source} ↔ ${target} iki yönlü`
            : undefined,
    }
  })

  return { nodes, edges }
}

// Maps SwarmGo's relationship graphs (workspace collaboration network and the
// per-agent memory knowledge graph) into vis-network node/edge data — the same
// library Agent-MCP's dashboard uses, so layout/physics are handled by its
// engine (see components/graph/VisNetworkGraph).
import type { Node, Edge } from 'vis-network'
import type {
  WorkspaceGraph,
  WorkspaceGraphEdge,
  WorkspaceNodeType,
  MemoryGraph,
} from '../types'

// Edge colors per workspace relationship kind, so the network reads at a glance.
const EDGE_COLOR: Record<WorkspaceGraphEdge['kind'], string> = {
  owns: '#10b981', // emerald — agent owns task
  created: '#f59e0b', // amber — agent authored task
  runs: '#7c3aed', // violet — task runs a flow
  uses: '#0ea5e9', // sky — flow uses agent
  skill: '#eab308', // yellow — agent uses skill
  mcp: '#14b8a6', // teal — agent ↔ MCP server
}

export const EDGE_LEGEND: { kind: WorkspaceGraphEdge['kind']; label: string; color: string }[] = [
  { kind: 'owns', label: 'sahip', color: EDGE_COLOR.owns },
  { kind: 'created', label: 'oluşturdu', color: EDGE_COLOR.created },
  { kind: 'runs', label: 'akış çalıştırır', color: EDGE_COLOR.runs },
  { kind: 'uses', label: 'ajan kullanır', color: EDGE_COLOR.uses },
]

// Toggleable node layers (agents are always shown). Drives the toolbar chips.
export const NODE_LAYERS: { type: WorkspaceNodeType; label: string; color: string }[] = [
  { type: 'task', label: 'Görevler', color: '#64748b' },
  { type: 'flow', label: 'Akışlar', color: '#7c3aed' },
  { type: 'skill', label: 'Beceriler', color: '#eab308' },
  { type: 'mcp', label: 'MCP', color: '#14b8a6' },
]

// Memory-kind tints for the knowledge-graph nodes.
export const MEMORY_KIND_COLOR: Record<string, string> = {
  document: '#2563eb', // blue
  journal: '#64748b', // slate
  reflection: '#059669', // emerald
}

// Board-state tints for task nodes.
const STATUS_COLOR: Record<string, string> = {
  todo: '#64748b',
  in_progress: '#0ea5e9',
  review: '#f59e0b',
  done: '#10b981',
  failed: '#ef4444',
}


function truncate(s: string, max = 28): string {
  const t = s.replace(/\s+/g, ' ').trim()
  return t.length > max ? t.slice(0, max - 1) + '…' : t
}

// esc HTML-escapes user text for safe insertion into a tooltip element.
function esc(s: string): string {
  const d = document.createElement('div')
  d.textContent = s
  return d.innerHTML
}

// tip builds a rich hover tooltip element (vis-network accepts an HTMLElement as
// a node's `title`): a bold heading plus optional dimmer detail lines.
function tip(heading: string, lines: (string | undefined)[]): HTMLElement {
  const el = document.createElement('div')
  el.style.maxWidth = '320px'
  el.style.whiteSpace = 'normal'
  el.style.lineHeight = '1.35'
  el.innerHTML =
    `<div style="font-weight:600;margin-bottom:3px">${esc(heading)}</div>` +
    lines
      .filter((l): l is string => !!l && l.trim() !== '')
      .map((l) => `<div style="opacity:.75;font-size:12px">${esc(l)}</div>`)
      .join('')
  return el
}

const STATUS_LABEL: Record<string, string> = {
  todo: 'Yapılacak',
  in_progress: 'Sürüyor',
  review: 'İncelemede',
  done: 'Tamamlandı',
  failed: 'Başarısız',
}

export interface VisData {
  nodes: Node[]
  edges: Edge[]
}

// workspaceToVis maps agents/tasks/flows/skills/mcp to vis-network nodes (agent =
// colored disc, task = status-bordered box, flow = violet diamond, skill = yellow
// hexagon, mcp = teal square) and the relationships to directed, color-coded
// edges. `visible` (a set of node types) filters layers; agents are always shown,
// and edges touching a hidden node are dropped.
export function workspaceToVis(graph: WorkspaceGraph, visible?: Set<WorkspaceNodeType>): VisData {
  const show = (t: WorkspaceNodeType) => t === 'agent' || !visible || visible.has(t)
  const shownIds = new Set(graph.nodes.filter((n) => show(n.type)).map((n) => n.id))
  const nodes: Node[] = graph.nodes
    .filter((n) => show(n.type))
    .map((n) => {
    if (n.type === 'agent') {
      const c = n.color || '#7c3aed'
      return {
        id: n.id,
        label: (n.emoji ? n.emoji + ' ' : '') + n.label,
        title: n.sub ? `${n.label} · ${n.sub}` : n.label,
        shape: 'dot',
        size: 24,
        color: { background: c, border: c, highlight: { background: c, border: '#fff' } },
        font: { color: '#f1f5f9', size: 14, strokeWidth: 3, strokeColor: '#0b0e14' },
      }
    }
    if (n.type === 'flow') {
      return {
        id: n.id,
        label: truncate(n.label, 22),
        title: n.sub ? `${n.label} — ${n.sub}` : n.label,
        shape: 'diamond',
        size: 18,
        color: { background: '#7c3aed', border: '#a78bfa', highlight: { background: '#8b5cf6', border: '#fff' } },
        font: { color: '#e9d5ff', size: 12 },
      }
    }
    if (n.type === 'skill') {
      // Skills get a distinct star icon with the slug as a small label below.
      return {
        id: n.id,
        label: truncate(n.label, 20),
        title: tip(n.label, ['Beceri (skill)']),
        shape: 'star',
        size: 14,
        color: { background: '#eab308', border: '#fde047', highlight: { background: '#facc15', border: '#fff' } },
        font: { color: '#fde68a', size: 11 },
      }
    }
    if (n.type === 'mcp') {
      // MCP servers as triangles (kept distinct from the task square).
      return {
        id: n.id,
        label: truncate(n.label, 20),
        title: tip(n.label, [n.sub ? `MCP sunucusu · ${n.sub}` : 'MCP sunucusu']),
        shape: 'triangle',
        size: 15,
        color: { background: '#14b8a6', border: '#5eead4', highlight: { background: '#2dd4bf', border: '#fff' } },
        font: { color: '#99f6e4', size: 11 },
      }
    }
    // task: a status-colored square with the title below; the full description
    // (and status) shows on hover via a rich tooltip.
    const sc = STATUS_COLOR[n.status ?? ''] ?? '#64748b'
    return {
      id: n.id,
      label: truncate(n.label, 22),
      title: tip(n.label, [
        n.status ? `Durum: ${STATUS_LABEL[n.status] ?? n.status}` : undefined,
        n.desc,
      ]),
      shape: 'square',
      size: 14,
      color: { background: sc, border: sc, highlight: { background: sc, border: '#fff' } },
      font: { color: '#cbd5e1', size: 11 },
    }
  })

  const edges: Edge[] = graph.edges
    .filter((e) => shownIds.has(e.source) && shownIds.has(e.target))
    .map((e, i) => {
      const color = EDGE_COLOR[e.kind]
      return {
        id: `e${i}`,
        from: e.source,
        to: e.target,
        color: { color, highlight: color, opacity: 0.85 },
        width: 1.6,
        arrows: { to: { enabled: true, scaleFactor: 0.6 } },
        dashes: e.kind === 'created',
      }
    })

  return { nodes, edges }
}

// memoryToVis maps each memory to a kind-colored dot sized by its similarity
// degree (hubs read larger) and each similarity pair to an undirected edge whose
// width/opacity scales with the cosine score.
export function memoryToVis(graph: MemoryGraph): VisData {
  const nodes: Node[] = graph.nodes.map((n) => {
    const c = MEMORY_KIND_COLOR[n.kind] ?? '#64748b'
    return {
      id: n.id,
      title: n.content,
      shape: 'dot',
      size: 8 + Math.min(20, n.degree * 3),
      color: { background: c, border: c, highlight: { background: c, border: '#fff' } },
    }
  })

  const edges: Edge[] = graph.edges.map((e, i) => ({
    id: `m${i}`,
    from: e.source,
    to: e.target,
    color: { color: '#8b5cf6', opacity: Math.min(0.9, 0.25 + e.score) },
    width: 0.6 + e.score * 3,
    smooth: { enabled: true, type: 'continuous', roundness: 0.5 },
  }))

  return { nodes, edges }
}

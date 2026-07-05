// Maps TionSwarm's relationship graphs (workspace collaboration network and the
// per-agent memory knowledge graph) into vis-network node/edge data — the same
// library Agent-MCP's dashboard uses, so layout/physics are handled by its
// engine (see components/graph/VisNetworkGraph).
import type { Node, Edge } from 'vis-network'
import type {
  WorkspaceGraph,
  WorkspaceGraphEdge,
  WorkspaceNodeType,
  MemoryGraph,
  BoardColumnDef,
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
  { type: 'skill', label: 'Skills', color: '#eab308' },
  { type: 'mcp', label: 'MCP', color: '#14b8a6' },
  { type: 'run', label: 'Geçmiş', color: '#52525b' },
]

// Completed-run node colors by execution kind (live "Geçmiş" archive).
const RUN_KIND_COLOR: Record<string, string> = {
  chat: '#3b82f6', // blue
  task: '#64748b', // slate
  flow: '#7c3aed', // violet
  schedule: '#0891b2', // cyan
}
const RUN_KIND_LABEL: Record<string, string> = {
  chat: 'Sohbet',
  task: 'Görev çalıştırması',
  flow: 'Akış çalıştırması',
  schedule: 'Zamanlama teslimi',
}

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


// groupHue maps an arbitrary group label to a stable HSL color (deterministic
// hash → hue), so every skill sharing a group gets the same tint across renders.
function groupHue(label: string, lightness: number): string {
  let h = 0
  for (let i = 0; i < label.length; i++) h = (h * 31 + label.charCodeAt(i)) >>> 0
  return `hsl(${h % 360}, 62%, ${lightness}%)`
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

export type WorkspaceMode = 'relation' | 'live'

// Board-state columns for the live mode: fixed anchor headers across the top that
// tasks gather under. Order = kanban flow left→right.
export const BOARD_COLUMNS: { state: string; label: string; color: string }[] = [
  { state: 'todo', label: 'Yapılacak', color: '#64748b' },
  { state: 'in_progress', label: 'Sürüyor', color: '#0ea5e9' },
  { state: 'review', label: 'İncelemede', color: '#f59e0b' },
  { state: 'done', label: 'Tamamlandı', color: '#10b981' },
  { state: 'failed', label: 'Başarısız', color: '#ef4444' },
]
const COL_PREFIX = 'col:'
const COL_GAP = 360 // horizontal spacing between column anchors
const COL_Y = -380 // anchors sit at the top of the canvas
const IDLE_ID = 'idle' // lobby anchor that taskless agents drift to
const IDLE_X = -320 // bottom-left
const IDLE_Y = 440 // bottom of the canvas, opposite the columns
const HIST_ID = 'history' // archive anchor that completed runs pile up at
const HIST_X = 620 // bottom-right, separated from the idle lobby
const HIST_Y = 440

// nodeFor builds the vis node for one workspace graph entity (shared by both
// modes).
// colColor (when provided) maps a board-column key → its configured color, so
// task nodes pick up their column's hue instead of the hard-coded STATUS_COLOR
// fallback (which only knows the five built-in statuses).
function nodeFor(n: WorkspaceGraph['nodes'][number], colColor?: Map<string, string>): Node {
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
      // When a skill carries an organisation group (n.sub), same-group skills are
      // tinted with a shared, group-derived hue so clusters read at a glance;
      // ungrouped skills keep the default yellow.
      const grouped = !!n.sub
      const bg = grouped ? groupHue(n.sub!, 58) : '#eab308'
      const border = grouped ? groupHue(n.sub!, 72) : '#fde047'
      return {
        id: n.id,
        label: truncate(n.label, 20),
        title: tip(n.label, [n.sub ? `Skill · ${n.sub}` : 'Skill']),
        shape: 'star',
        size: 14,
        color: { background: bg, border, highlight: { background: border, border: '#fff' } },
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
    if (n.type === 'run') {
      // Completed run (archive): a titled card like the Activity/kanban entries —
      // a kind-colored bordered box showing the run title; kind + agent on hover.
      const c = RUN_KIND_COLOR[n.runKind ?? ''] ?? '#52525b'
      return {
        id: n.id,
        label: truncate(n.label, 26),
        title: tip(n.label, [RUN_KIND_LABEL[n.runKind ?? ''] ?? 'Çalıştırma', n.sub ? `Ajan: ${n.sub}` : undefined]),
        shape: 'box',
        color: { background: 'rgba(24,24,27,0.95)', border: c, highlight: { background: '#27272a', border: c } },
        font: { color: '#d4d4d8', size: 11 },
        shapeProperties: { borderRadius: 6 },
        margin: { top: 5, bottom: 5, left: 9, right: 9 } as Node['margin'],
      }
    }
    // task: a board-colored square with the title below; the full description
    // (and status) shows on hover via a rich tooltip. The square's color follows
    // the task's board column (colColor), falling back to the built-in status
    // tint and finally a neutral slate.
    const sc = colColor?.get(n.status ?? '') ?? STATUS_COLOR[n.status ?? ''] ?? '#64748b'
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
}

// edgeId is a stable, content-derived id so incremental DataSet updates can diff
// edges across refreshes (an unchanged edge keeps its id → no flicker).
function edgeId(kind: string, from: string, to: string): string {
  return `${kind}:${from}->${to}`
}

// workspaceToVis maps the workspace graph to vis-network data. `mode`:
//  - 'relation' (default): the full collaboration web — every node type + all
//    relationship edges (owns/created/runs/uses/skill/mcp).
//  - 'live': a kanban-flow view — fixed board-state column anchors across the
//    top, every task springs to its column, and an agent bonds ONLY to the task
//    it is actively working (in_progress + owner). Skills/MCP stay bonded to the
//    agent so they drift with it. As board states change the bonds re-form,
//    giving the "living flow" when combined with live refresh.
// `visible` filters the skill/mcp layers (agents/tasks/columns always shown).
export function workspaceToVis(
  graph: WorkspaceGraph,
  visible?: Set<WorkspaceNodeType>,
  mode: WorkspaceMode = 'relation',
  // User-defined Kanban columns (saved in WorkspaceSettings.boardColumns). When
  // supplied in live mode these REPLACE the hard-coded BOARD_COLUMNS set so
  // newly added / renamed / reordered columns appear on the Network screen.
  // Falls back to the static defaults when omitted.
  boardColumns?: BoardColumnDef[],
): VisData {
  // Effective live-mode column anchors: user-defined when non-empty, else the
  // built-in defaults (a workspace with no saved columns degrades to the 5
  // standard statuses rather than going column-less).
  const liveColumns: { state: string; label: string; color: string }[] =
    boardColumns && boardColumns.length > 0
      ? boardColumns.map((c) => ({
          state: c.key,
          label: c.label,
          color: c.color || STATUS_COLOR[c.key] || '#64748b',
        }))
      : BOARD_COLUMNS
  // Per-column color lookup so task nodes inherit their board column's hue.
  const colColor = new Map(liveColumns.map((c) => [c.state, c.color]))
  const live = mode === 'live'
  // Visibility: agents always; tasks always in live; flow/skill/mcp by toggle.
  const show = (t: WorkspaceNodeType): boolean => {
    if (t === 'agent') return true
    if (t === 'run') return live && (!visible || visible.has('run')) // archive: live only
    if (live && t === 'task') return true
    return !visible || visible.has(t)
  }
  const shownIds = new Set(graph.nodes.filter((n) => show(n.type)).map((n) => n.id))
  const statusOf = new Map(graph.nodes.filter((n) => n.type === 'task').map((n) => [n.id, n.status]))

  const nodes: Node[] = graph.nodes.filter((n) => show(n.type)).map((n) => nodeFor(n, colColor))

  const edges: Edge[] = []
  const addEdge = (kind: string, from: string, to: string, style: Partial<Edge>) => {
    if (!shownIds.has(from) || !shownIds.has(to)) return
    edges.push({ id: edgeId(kind, from, to), from, to, ...style } as Edge)
  }

  if (live) {
    // Fixed column anchors across the top -- driven by the user's Kanban
    // columns (boardColumns), falling back to the static defaults.
    const n = liveColumns.length
    liveColumns.forEach((col, i) => {
      const x = (i - (n - 1) / 2) * COL_GAP
      nodes.push({
        id: COL_PREFIX + col.state,
        label: col.label,
        shape: 'box',
        x,
        y: COL_Y,
        // physics:false (without `fixed`) → the solver never moves it, but the
        // user can still drag it and it stays put.
        physics: false,
        color: { background: 'rgba(30,39,51,0.9)', border: col.color },
        font: { color: col.color, size: 15, bold: { color: col.color } } as Node['font'],
        margin: { top: 8, bottom: 8, left: 14, right: 14 } as Node['margin'],
        widthConstraint: { minimum: 110 } as Node['widthConstraint'],
      })
    })
    // Every task springs to its board-state column.
    for (const t of graph.nodes) {
      if (t.type !== 'task') continue
      const col = COL_PREFIX + (t.status || 'todo')
      edges.push({
        id: edgeId('col', t.id, col),
        from: t.id,
        to: col,
        color: { color: '#334155', opacity: 0.5 },
        width: 1,
        length: 150,
        dashes: true,
        smooth: false,
      } as Edge)
    }
    // Idle lobby anchor (bottom-left) — taskless agents drift here.
    nodes.push({
      id: IDLE_ID,
      label: 'Boşta',
      shape: 'box',
      x: IDLE_X,
      y: IDLE_Y,
      physics: false, // immune to forces, but user-draggable

      color: { background: 'rgba(30,39,51,0.7)', border: '#475569' },
      font: { color: '#94a3b8', size: 13 } as Node['font'],
      margin: { top: 6, bottom: 6, left: 14, right: 14 } as Node['margin'],
      widthConstraint: { minimum: 90 } as Node['widthConstraint'],
    })

    // History/archive anchor (bottom-right) — completed runs pile up here.
    const hasRuns = nodes.some((nd) => (nd.id as string).startsWith('run:'))
    if (hasRuns) {
      nodes.push({
        id: HIST_ID,
        label: 'Geçmiş',
        shape: 'box',
        x: HIST_X,
        y: HIST_Y,
        physics: false, // immune to forces, but user-draggable

        color: { background: 'rgba(30,39,51,0.7)', border: '#52525b' },
        font: { color: '#a1a1aa', size: 13 } as Node['font'],
        margin: { top: 6, bottom: 6, left: 14, right: 14 } as Node['margin'],
        widthConstraint: { minimum: 90 } as Node['widthConstraint'],
      })
      // Each completed run springs to the archive anchor.
      for (const n of graph.nodes) {
        if (n.type !== 'run' || !shownIds.has(n.id)) continue
        edges.push({
          id: edgeId('hist', n.id, HIST_ID),
          from: n.id,
          to: HIST_ID,
          color: { color: '#3f3f46', opacity: 0.4 },
          width: 0.8,
          length: 120,
          dashes: true,
          smooth: false,
        } as Edge)
      }
    }

    // Active bonds come from two signals: a live in-flight run (agent.running +
    // runTarget — the strongest "doing it right now") and, as a fallback, owning
    // an in_progress task. Collect the (agentId → targetNodeId) pairs.
    const activeTarget = new Map<string, string>()
    for (const a of graph.nodes) {
      if (a.type === 'agent' && a.running && a.runTarget) activeTarget.set(a.id, a.runTarget)
    }
    for (const e of graph.edges) {
      if (e.kind === 'owns' && statusOf.get(e.target) === 'in_progress' && !activeTarget.has(e.source)) {
        activeTarget.set(e.source, e.target)
      }
    }
    // Busy = has an active target OR is running anything (chat/schedule glow).
    const runningAgents = new Set(graph.nodes.filter((n) => n.type === 'agent' && n.running).map((n) => n.id))
    const busyAgents = new Set<string>([...activeTarget.keys(), ...runningAgents])
    // Idle agents drift to the lobby via a weak spring (an active task bond, when
    // present, easily overpowers it and pulls the agent up to its card).
    for (const a of graph.nodes) {
      if (a.type !== 'agent' || busyAgents.has(a.id)) continue
      edges.push({
        id: edgeId('idle', a.id, IDLE_ID),
        from: a.id,
        to: IDLE_ID,
        color: { color: '#475569', opacity: 0.35 },
        width: 1,
        length: 220,
        dashes: true,
        smooth: false,
      } as Edge)
    }

    // Active bonds: agent → the task/flow it is running (or owns in_progress).
    for (const [agentId, target] of activeTarget) {
      addEdge('active', agentId, target, {
        color: { color: 'var(--color-accent)', highlight: '#fff', opacity: 1 },
        width: 3,
        arrows: { to: { enabled: true, scaleFactor: 0.7 } },
        shadow: { enabled: true, color: 'var(--color-accent)', size: 12, x: 0, y: 0 } as Edge['shadow'],
      })
    }
    // Agent attachments: skills, MCP servers and the flow(s) it is wired into.
    for (const e of graph.edges) {
      if (e.kind === 'skill' || e.kind === 'mcp' || e.kind === 'uses') {
        const color = EDGE_COLOR[e.kind]
        addEdge(e.kind, e.source, e.target, {
          color: { color, highlight: color, opacity: 0.7 },
          width: 1.2,
        })
      }
    }
    // Give running agents a bright "live" glow so it's clear who's working now.
    if (runningAgents.size > 0) {
      const byId = new Map(nodes.map((nd) => [nd.id as string, nd]))
      for (const id of runningAgents) {
        const nd = byId.get(id)
        if (!nd) continue
        nd.borderWidth = 3
        nd.color = { background: (nd.color as { background?: string })?.background ?? '#7c3aed', border: '#fff' }
        nd.shadow = { enabled: true, color: 'var(--color-accent)', size: 22, x: 0, y: 0 } as Node['shadow']
      }
    }
    return { nodes, edges }
  }

  // relation mode: the full relationship web.
  for (const e of graph.edges) {
    const color = EDGE_COLOR[e.kind]
    addEdge(e.kind, e.source, e.target, {
      color: { color, highlight: color, opacity: 0.85 },
      width: 1.6,
      arrows: { to: { enabled: true, scaleFactor: 0.6 } },
      dashes: e.kind === 'created',
    })
  }
  return { nodes, edges }
}

export const MEMORY_KIND_LABEL: Record<string, string> = {
  document: 'Belge',
  journal: 'Günlük',
  reflection: 'Yansıma',
}

// fmtDate formats a unix-seconds timestamp as a short tr-TR date+time (app
// context, so Date is available). Empty when missing.
export function fmtDate(sec: number): string {
  if (!sec) return ''
  try {
    return new Date(sec * 1000).toLocaleString('tr-TR', { dateStyle: 'short', timeStyle: 'short' })
  } catch {
    return ''
  }
}

// Memory-kind anchors (draggable, physics-immune) the memories spring to when
// the "kind anchors" layout is on — clusters memories by kind, like the live
// board columns.
const MEMORY_KINDS = ['document', 'journal', 'reflection'] as const
const MEM_ANCHOR_PREFIX = 'mk:'
const MEM_ANCHOR_GAP = 300 // horizontal spacing between kind anchors
const MEM_ANCHOR_Y = -340 // anchors sit across the top

// clusterHue spreads component colors around the wheel via the golden angle so
// adjacent cluster indices stay visually distinct.
function clusterHue(i: number): string {
  return `hsl(${Math.round((i * 137.508) % 360)}, 62%, 58%)`
}

// connectedComponents runs union-find over the similarity edges and returns each
// node id → its component root (so topic groups can be colored together).
function connectedComponents(
  ids: string[],
  edges: { source: string; target: string }[],
): Map<string, string> {
  const parent = new Map<string, string>()
  ids.forEach((id) => parent.set(id, id))
  const find = (x: string): string => {
    let r = x
    while (parent.get(r) !== r) r = parent.get(r)!
    while (parent.get(x) !== r) {
      const nxt = parent.get(x)!
      parent.set(x, r)
      x = nxt
    }
    return r
  }
  for (const e of edges) {
    if (!parent.has(e.source) || !parent.has(e.target)) continue
    const ra = find(e.source)
    const rb = find(e.target)
    if (ra !== rb) parent.set(ra, rb)
  }
  const root = new Map<string, string>()
  ids.forEach((id) => root.set(id, find(id)))
  return root
}

export interface MemoryVisOptions {
  // Add draggable per-kind anchors and spring each memory to its kind anchor.
  kindAnchors?: boolean
  // Color connected components (topic groups) with distinct hues instead of
  // tinting purely by kind. Singletons keep their kind color.
  clusterColor?: boolean
}

// memoryToVis maps each memory to a node carrying a short content preview label
// (so memories are tellable apart at a glance) + a rich hover tooltip (kind +
// content + date). Shape/size encode kind & importance: reflections (high-level
// summaries) are larger stars, documents/journals are discs sized by their
// similarity degree (hubs read bigger). Edges scale width/opacity with the
// cosine score. Optional layouts: per-kind draggable anchors (cluster by kind)
// and connected-component coloring (cluster by topic).
export function memoryToVis(graph: MemoryGraph, opts: MemoryVisOptions = {}): VisData {
  const { kindAnchors = false, clusterColor = false } = opts

  // Connected-component coloring: assign a distinct hue to every component with
  // ≥2 members (singletons keep their kind color, so loners aren't miscolored).
  let compColor: Map<string, string> | null = null
  if (clusterColor) {
    const ids = graph.nodes.map((n) => n.id)
    const root = connectedComponents(ids, graph.edges)
    const size = new Map<string, number>()
    root.forEach((r) => size.set(r, (size.get(r) ?? 0) + 1))
    const palette = new Map<string, string>()
    let idx = 0
    for (const id of ids) {
      const r = root.get(id)!
      if ((size.get(r) ?? 0) >= 2 && !palette.has(r)) palette.set(r, clusterHue(idx++))
    }
    compColor = new Map()
    root.forEach((r, id) => {
      const c = palette.get(r)
      if (c) compColor!.set(id, c)
    })
  }

  const nodes: Node[] = graph.nodes.map((n) => {
    const reflection = n.kind === 'reflection'
    const base = reflection ? 16 : n.kind === 'document' ? 13 : 10
    const cc = compColor?.get(n.id)
    const c = cc ?? MEMORY_KIND_COLOR[n.kind] ?? '#64748b'
    return {
      id: n.id,
      label: truncate(n.content, 22),
      title: tip(truncate(n.content, 70), [MEMORY_KIND_LABEL[n.kind] ?? n.kind, fmtDate(n.createdAt)]),
      shape: reflection ? 'star' : 'dot',
      size: base + Math.min(16, n.degree * 2),
      color: { background: c, border: reflection ? '#fff' : c, highlight: { background: c, border: '#fff' } },
      font: { color: '#cbd5e1', size: 11 },
    }
  })

  const edges: Edge[] = graph.edges.map((e, i) => {
    const cc = compColor?.get(e.source)
    return {
      id: `m${i}`,
      from: e.source,
      to: e.target,
      title: `benzerlik: ${(e.score * 100).toFixed(0)}%`,
      color: { color: cc ?? '#8b5cf6', highlight: cc ?? '#c4b5fd', opacity: Math.min(0.9, 0.25 + e.score) },
      width: 0.6 + e.score * 3,
      smooth: { enabled: true, type: 'continuous', roundness: 0.5 },
    }
  })

  // Kind anchors: draggable, physics-immune boxes the memories spring toward,
  // grouping the cloud by kind (same mechanic as the live board columns).
  if (kindAnchors) {
    const present = MEMORY_KINDS.filter((k) => graph.nodes.some((n) => n.kind === k))
    const n = present.length
    present.forEach((k, i) => {
      const x = (i - (n - 1) / 2) * MEM_ANCHOR_GAP
      nodes.push({
        id: MEM_ANCHOR_PREFIX + k,
        label: MEMORY_KIND_LABEL[k],
        shape: 'box',
        x,
        y: MEM_ANCHOR_Y,
        physics: false, // immune to forces, but user-draggable
        color: { background: 'rgba(30,39,51,0.9)', border: MEMORY_KIND_COLOR[k] },
        font: { color: MEMORY_KIND_COLOR[k], size: 14, bold: { color: MEMORY_KIND_COLOR[k] } } as Node['font'],
        margin: { top: 8, bottom: 8, left: 14, right: 14 } as Node['margin'],
        widthConstraint: { minimum: 100 } as Node['widthConstraint'],
      })
    })
    for (const m of graph.nodes) {
      edges.push({
        id: `mk-${m.id}`,
        from: m.id,
        to: MEM_ANCHOR_PREFIX + m.kind,
        color: { color: '#334155', opacity: 0.4 },
        width: 1,
        length: 170,
        dashes: true,
        smooth: false,
      } as Edge)
    }
  }

  return { nodes, edges }
}

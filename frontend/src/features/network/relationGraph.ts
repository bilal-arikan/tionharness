// Maps TionHarness's workspace collaboration network into vis-network node/edge
// data — the same library Agent-MCP's dashboard uses, so layout/physics are
// handled by its engine (see components/graph/VisNetworkGraph).
import type { Node, Edge } from 'vis-network'
import type { WorkspaceGraph, WorkspaceGraphEdge, WorkspaceNodeType, BoardColumnDef } from '@/types'
import { avatarForeground } from '@/shared/lib/avatar'

interface GraphTheme {
  bg: string
  surface: string
  surface2: string
  border: string
  text: string
  textDim: string
  accent: string
  onAccent: string
}

function graphTheme(): GraphTheme {
  const styles = getComputedStyle(document.documentElement)
  const color = (token: string): string => {
    const value = styles.getPropertyValue(token).trim()
    if (!value) throw new Error(`Missing graph theme token: ${token}`)
    return value
  }
  return {
    bg: color('--color-bg'),
    surface: color('--color-surface'),
    surface2: color('--color-surface-2'),
    border: color('--color-border'),
    text: color('--color-text'),
    textDim: color('--color-text-dim'),
    accent: color('--color-accent'),
    onAccent: color('--color-on-accent'),
  }
}

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
  { type: 'run', label: 'Oturumlar', color: '#52525b' },
]

// Completed-run node colors by execution kind (live "Geçmiş" archive).
const RUN_KIND_COLOR: Record<string, string> = {
  chat: '#3b82f6', // blue
  task: '#64748b', // slate
  flow: '#7c3aed', // violet
  'flow-coordinator': '#8b5cf6', // violet-light
  schedule: '#0891b2', // cyan
  worker: '#0d9488', // teal
  spawned: '#d97706', // amber
  inbox: '#db2777', // pink
}
const RUN_KIND_LABEL: Record<string, string> = {
  chat: 'Sohbet',
  task: 'Görev çalıştırması',
  flow: 'Akış çalıştırması',
  'flow-coordinator': 'Akış koordinatörü',
  schedule: 'Zamanlama teslimi',
  worker: 'Worker çalıştırması',
  spawned: 'Spawn çalıştırması',
  inbox: 'Inbox',
}

const LIVE_SCOPE_LABEL = {
  running: 'Çalışan',
  'awaiting-workers': 'Worker Bekleyen',
} as const

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

// initialsAscii returns a 1-2 char ASCII fallback for an agent's avatar when no
// emoji is stored. A high-codepoint emoji glyph is treated as already-rendered
// and bypassed (we don't want to half-show a broken glyph).
function initialsAscii(label: string): string {
  const parts = label.trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[1][0]).toUpperCase()
}

// agentAvatarDataUrl renders an SVG circular avatar (color-filled disc with
// the agent's emoji or initials centered) as a data URL. We hand vis-network
// this instead of `shape: 'dot'` so the node visual IS the agent's identity
// glyph rather than a generic colored circle. Emoji render via the system font
// when available; chromium-edge delivers consistent emoji across desktop OSes.
function agentAvatarDataUrl(emoji: string, color: string, size = 96): string {
  // Escape only what XML/URI needs: `&` to `&amp;`, `"` to `&quot;`. Browsers
  // also tolerate raw emoji bytes inside the SVG; encodeURIComponent over the
  // whole string keeps data URLs transport-safe (commas, quotes, `#`, etc.).
  const safeColor = color.replace(/"/g, '')
  const safeGlyph = emoji.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  const svg =
    `<svg xmlns="http://www.w3.org/2000/svg" width="${size}" height="${size}" viewBox="0 0 ${size} ${size}">` +
    `<defs><linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="100%">` +
    `<stop offset="0%" stop-color="${safeColor}"/>` +
    `<stop offset="100%" stop-color="${safeColor}" stop-opacity="0.72"/>` +
    `</linearGradient></defs>` +
    `<circle cx="${size / 2}" cy="${size / 2}" r="${size / 2}" fill="url(#g)" ` +
    `stroke="${safeColor}" stroke-width="2"/>` +
    `<text x="50%" y="50%" text-anchor="middle" dominant-baseline="central" ` +
    `font-size="${size * 0.5}" fill="${avatarForeground(safeColor)}" font-family="system-ui, -apple-system, 'Segoe UI Emoji', 'Noto Color Emoji', sans-serif" ` +
    `font-weight="600">${safeGlyph}</text>` +
    `</svg>`
  return 'data:image/svg+xml;utf8,' + encodeURIComponent(svg)
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
function nodeFor(
  n: WorkspaceGraph['nodes'][number],
  theme: GraphTheme,
  colColor?: Map<string, string>,
): Node {
  if (n.type === 'agent') {
    const c = n.color || '#7c3aed'
    // The agent's identity glyph is baked into the node image; label only
    // carries the name. Fall back to initials when no emoji is stored (and
    // we don't try to render broken / mojibake strings as glyphs here — the
    // upstream backend passes pre-cleaned emoji only).
    const glyph = n.emoji && n.emoji.trim() !== '' ? n.emoji : initialsAscii(n.label)
    const avatar = agentAvatarDataUrl(glyph, c)
    // Every agent node is a live INSTANCE (one per running session), so the
    // same agent can appear several times. The run kind goes on a second
    // label line to tell the copies apart; the full subtitle (kind + session
    // title) stays in the tooltip.
    const kind = n.liveScope ? LIVE_SCOPE_LABEL[n.liveScope] : n.sub ? n.sub.split(' · ')[0] : ''
    return {
      id: n.id,
      label: kind ? `${n.label}\n${kind}` : n.label,
      title: tip(n.label, [
        n.liveScope ? `Durum: ${LIVE_SCOPE_LABEL[n.liveScope]}` : undefined,
        n.sub,
        n.sessionId ? `Oturum: ${n.sessionId}` : undefined,
        '↗ oturumu açmak için tıkla',
      ]),
      shape: 'circularImage',
      size: 28,
      image: avatar,
      brokenImage: avatar,
      color: { background: c, border: c, highlight: { background: c, border: theme.text } },
      font: {
        color: theme.text,
        size: 14,
        strokeWidth: 3,
        strokeColor: theme.bg,
      },
    }
  }
  if (n.type === 'flow') {
    return {
      id: n.id,
      label: truncate(n.label, 22),
      title: n.sub ? `${n.label} — ${n.sub}` : n.label,
      shape: 'diamond',
      size: 18,
      color: {
        background: '#7c3aed',
        border: '#a78bfa',
        highlight: { background: '#8b5cf6', border: '#fff' },
      },
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
      color: {
        background: '#14b8a6',
        border: '#5eead4',
        highlight: { background: '#2dd4bf', border: '#fff' },
      },
      font: { color: '#99f6e4', size: 11 },
    }
  }
  if (n.type === 'run') {
    // Live session: a titled card with its authoritative live-scope status.
    const c = RUN_KIND_COLOR[n.runKind ?? ''] ?? '#52525b'
    const archived = !!n.archived
    return {
      id: n.id,
      label:
        (archived ? '🗄 ' : '') +
        truncate(n.label, archived ? 24 : 26) +
        (n.liveScope ? `\n${LIVE_SCOPE_LABEL[n.liveScope]}` : ''),
      title: tip(n.label, [
        n.liveScope ? `Durum: ${LIVE_SCOPE_LABEL[n.liveScope]}` : undefined,
        RUN_KIND_LABEL[n.runKind ?? ''] ?? 'Çalıştırma',
        n.sub ? `Ajan: ${n.sub}` : undefined,
        archived ? 'Arşivlenmiş' : undefined,
        '↗ oturumu açmak için tıkla',
      ]),
      shape: 'box',
      color: {
        background: archived ? theme.surface : theme.surface2,
        border: c,
        highlight: { background: theme.surface2, border: c },
      },
      font: { color: archived ? theme.textDim : theme.text, size: 11 },
      shapeProperties: { borderRadius: 6, borderDashes: archived ? [4, 3] : false },
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
  const theme = graphTheme()
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
  const statusOf = new Map(
    graph.nodes.filter((n) => n.type === 'task').map((n) => [n.id, n.status]),
  )

  const nodes: Node[] = graph.nodes
    .filter((n) => show(n.type))
    .map((n) => nodeFor(n, theme, colColor))

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
        color: { background: theme.surface2, border: col.color },
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
        color: { color: theme.border, opacity: 0.5 },
        width: 1,
        length: 150,
        dashes: true,
        smooth: false,
      } as Edge)
    }
    // Live-session anchor (bottom-right).
    const hasRuns = nodes.some((nd) => (nd.id as string).startsWith('run:'))
    if (hasRuns) {
      nodes.push({
        id: HIST_ID,
        label: 'Canlı Oturumlar',
        shape: 'box',
        x: HIST_X,
        y: HIST_Y,
        physics: false, // immune to forces, but user-draggable

        color: { background: theme.surface2, border: theme.border },
        font: { color: theme.textDim, size: 13 } as Node['font'],
        margin: { top: 6, bottom: 6, left: 14, right: 14 } as Node['margin'],
        widthConstraint: { minimum: 90 } as Node['widthConstraint'],
      })
      // Each eligible live session springs to the shared anchor.
      for (const n of graph.nodes) {
        if (n.type !== 'run' || !shownIds.has(n.id)) continue
        edges.push({
          id: edgeId('hist', n.id, HIST_ID),
          from: n.id,
          to: HIST_ID,
          color: { color: theme.border, opacity: 0.4 },
          width: 0.8,
          // Long spring so the many archive cards fan out into a wide ring around
          // the anchor rather than piling onto the same spot.
          length: 260,
          dashes: true,
          smooth: false,
        } as Edge)
      }
    }

    // Active bonds come from two signals: the instance's own run target (a task/
    // flow run — the strongest "doing it right now") and, as a fallback, owning
    // an in_progress task. Collect the (agent node id → targetNodeId) pairs.
    // Keys are INSTANCE ids, so two copies of the same agent can bond to two
    // different tasks at once.
    const activeTarget = new Map<string, string>()
    for (const a of graph.nodes) {
      if (a.type === 'agent' && a.runTarget) activeTarget.set(a.id, a.runTarget)
    }
    for (const e of graph.edges) {
      if (
        e.kind === 'owns' &&
        statusOf.get(e.target) === 'in_progress' &&
        !activeTarget.has(e.source)
      ) {
        activeTarget.set(e.source, e.target)
      }
    }
    // Every agent node is a live instance, so all of them get the running glow.
    const runningAgents = new Set(
      graph.nodes.filter((n) => n.type === 'agent' && n.liveScope === 'running').map((n) => n.id),
    )
    // Instances without a task/flow target (chat, schedule, spawn, worker, inbox)
    // have nothing to bond to, so they drift to a shared "Çalışıyor" anchor
    // instead of floating loose. The anchor only appears when someone needs it.
    const untargeted = graph.nodes.filter((n) => n.type === 'agent' && !activeTarget.has(n.id))
    if (untargeted.length > 0) {
      nodes.push({
        id: IDLE_ID,
        label: 'Çalışıyor',
        shape: 'box',
        x: IDLE_X,
        y: IDLE_Y,
        physics: false, // immune to forces, but user-draggable
        color: { background: theme.surface2, border: theme.border },
        font: { color: theme.textDim, size: 13 } as Node['font'],
        margin: { top: 6, bottom: 6, left: 14, right: 14 } as Node['margin'],
        widthConstraint: { minimum: 90 } as Node['widthConstraint'],
      })
      for (const a of untargeted) {
        edges.push({
          id: edgeId('idle', a.id, IDLE_ID),
          from: a.id,
          to: IDLE_ID,
          color: { color: theme.border, opacity: 0.35 },
          width: 1,
          // Wide spring so running instances spread around the "Çalışıyor" core
          // instead of stacking.
          length: 260,
          dashes: true,
          smooth: false,
        } as Edge)
      }
    }

    // Active bonds: agent → the task/flow it is running (or owns in_progress).
    for (const [agentId, target] of activeTarget) {
      addEdge('active', agentId, target, {
        color: { color: theme.accent, highlight: theme.onAccent, opacity: 1 },
        width: 3,
        arrows: { to: { enabled: true, scaleFactor: 0.7 } },
        shadow: {
          enabled: true,
          color: theme.accent,
          size: 12,
          x: 0,
          y: 0,
        } as Edge['shadow'],
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
        nd.color = {
          background: (nd.color as { background?: string })?.background ?? '#7c3aed',
          border: theme.text,
        }
        nd.shadow = {
          enabled: true,
          color: theme.accent,
          size: 22,
          x: 0,
          y: 0,
        } as Node['shadow']
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

import type { Edge, Node } from 'vis-network'
import type { ViewGraphLive, ViewGraphResult, ViewHandle, ViewKind, ViewRef } from '@/types'
import { refToString } from '@/types'
import { CATEGORY_LABELS } from '@/features/tools/toolMeta'
import { agentAvatarDataUrl, initialsAscii } from '@/features/network/agentAvatar'
import { SESSION_KIND_LABEL } from './explorerFilter'
import { isLiveAgentRef } from './explorerLive'
import type { SeedLayout } from './explorerSeed'

// Maps the whole-workspace structural map (GET /api/views/graph) onto vis-network
// nodes/edges. Inherits the visual language of the former Ağ screen (session =
// box, flow = diamond, skill = star, MCP = triangle); the workspace root and its
// buckets are the hubs the force field arranges everything around.

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

// Sub-node roles: a ref's `sub` selects one slice of a hub node. The map gives
// each role its own shape and label so a board column, a card, a tool group, an
// MCP server and a billing provider never read as the same thing.
export type NodeRole =
  | 'board-column'
  | 'board-card'
  | 'tool-group'
  | 'tool-mcp'
  | 'budget-provider'
  | 'live-agent'
  | 'session-kind'
  | null

export function nodeRole(ref: ViewRef): NodeRole {
  if (isLiveAgentRef(ref)) return 'live-agent'
  if (ref.kind === 'category' && ref.id.startsWith('col:')) return 'board-column'
  if (ref.kind === 'category' && ref.id.startsWith('skind:')) return 'session-kind'
  if (ref.kind === 'board' && ref.sub) return 'board-card'
  if (ref.kind === 'tools' && ref.sub?.startsWith('group:')) return 'tool-group'
  if (ref.kind === 'tools' && ref.sub?.startsWith('mcp:')) return 'tool-mcp'
  if (ref.kind === 'budget' && ref.sub?.startsWith('provider:')) return 'budget-provider'
  return null
}

export const ROLE_LABEL: Record<Exclude<NodeRole, null>, string> = {
  'board-column': 'Pano sütunu',
  'board-card': 'Kart',
  'tool-group': 'Araç grubu',
  'tool-mcp': 'MCP sunucusu',
  'budget-provider': 'Sağlayıcı',
  'live-agent': 'Çalışan ajan',
  'session-kind': 'Oturum türü',
}

// nodeLabelOfKind is the human name of what a node is: the role label for a
// sub node, otherwise the kind label.
export function nodeLabelOfKind(ref: ViewRef): string {
  const role = nodeRole(ref)
  return role ? ROLE_LABEL[role] : KIND_LABEL[ref.kind]
}

export function kindColor(ref: ViewRef): string {
  if (ref.kind === 'workspace') return THEME_FALLBACK.accent
  if (ref.kind === 'category') {
    if (ref.id.startsWith('col:')) {
      return BOARD_COLUMN_COLOR[ref.id.slice('col:'.length)] ?? '#64748b'
    }
    if (ref.id.startsWith('skind:')) return '#38bdf8'
    return CATEGORY_COLOR[ref.id] ?? '#64748b'
  }
  switch (nodeRole(ref)) {
    case 'session-kind':
      return '#38bdf8'
    case 'tool-mcp':
      return '#14b8a6'
    case 'tool-group':
      return '#0d9488'
    case 'budget-provider':
      return '#16a34a'
    case 'board-card':
      return '#94a3b8'
    default:
      return KIND_COLOR[ref.kind] ?? '#64748b'
  }
}

// displayLabel strips the "kind:ID " spelling the backend prefixes handle labels
// with ("session:SES12 Fix login" → "Fix login"); the ref stays in the tooltip.
// Tool groups swap the backend's category key for the tools screen's Turkish
// label, so the map and the Araçlar screen name a group the same way.
export function displayLabel(handle: ViewHandle): string {
  const key = refToString(handle.ref)
  let label = handle.label.trim()
  if (nodeRole(handle.ref) === 'tool-group') {
    const groupKey = handle.ref.sub!.slice('group:'.length)
    const localized = CATEGORY_LABELS[groupKey]
    if (localized) label = label.replace(groupKey, localized)
    return label || groupKey
  }
  if (nodeRole(handle.ref) === 'session-kind') {
    const kind = handle.ref.id.slice('skind:'.length)
    const localized = SESSION_KIND_LABEL[kind] ?? (kind === 'other' ? 'Diğer' : undefined)
    if (localized) label = label.replace(kind, localized)
    return label || kind
  }
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

function shapeFor(ref: ViewRef, depth: number): Node['shape'] {
  if (depth <= 1) return 'dot'
  switch (nodeRole(ref)) {
    case 'board-column':
      return 'square'
    case 'board-card':
      return 'box'
    case 'tool-group':
      return 'hexagon'
    case 'tool-mcp':
      return 'triangle'
    case 'budget-provider':
      return 'dot'
    case 'live-agent':
      return 'circularImage'
    case 'session-kind':
      return 'dot'
    default:
      break
  }
  switch (ref.kind) {
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
  // Live layer (explorerLive): sessions executing now glow, and their avatar
  // nodes are drawn as the agent's picture.
  liveState?: ReadonlyMap<string, ViewGraphLive['state']>
  liveAgents?: ReadonlyMap<string, ViewGraphLive>
  // Folded nodes and how many children each hides: drawn with a "+N" badge.
  collapsed?: ReadonlySet<string>
  childCounts?: ReadonlyMap<string, number>
}

// Glow of a live session: a wide, tinted shadow plus a thick border in the
// agent's color. Running is full strength; a coordinator awaiting its workers
// glows softer in the warning hue.
function liveGlow(
  state: ViewGraphLive['state'],
  agentColor: string | undefined,
  theme: ExplorerTheme,
): Pick<Node, 'shadow' | 'borderWidth'> & { border: string } {
  const running = state === 'running'
  const border = running ? agentColor || theme.accent : theme.warning
  return {
    border,
    borderWidth: running ? 3 : 2.5,
    shadow: { enabled: true, color: border, size: running ? 28 : 18, x: 0, y: 0 },
  }
}

export interface ExplorerVisData {
  nodes: Node[]
  edges: Edge[]
}

// liveAgentColor finds the color of the agent driving a live session, through
// its avatar node (the only place the payload carries the agent's color).
function liveAgentColor(
  sessionKey: string,
  liveAgents: ReadonlyMap<string, ViewGraphLive>,
): string | undefined {
  for (const live of liveAgents.values()) {
    if (refToString(live.session) === sessionKey) return live.agent.color || undefined
  }
  return undefined
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
    const role = nodeRole(handle.ref)
    // Second-level groups (session kinds, board columns) sit between bucket and
    // member: a little bigger and heavier than a leaf so the tier reads.
    const { size, mass, fontSize } =
      role === 'session-kind' || role === 'board-column'
        ? { size: 16, mass: 2, fontSize: 12 }
        : depthStyle(depth)
    const shape = shapeFor(handle.ref, depth)
    const boxed = shape === 'box'
    // A board card is a dashed box in its column's grey; a session stays a
    // solid box, so the two box-shaped kinds still differ at a glance.
    const dashedBox = role === 'board-card'
    const seed = opts.layout.positions[key]
    const liveEntry = opts.liveAgents?.get(key)
    const liveSessionState = handle.ref.kind === 'session' ? opts.liveState?.get(key) : undefined
    const glow = liveSessionState
      ? liveGlow(
          liveSessionState,
          opts.liveAgents && liveAgentColor(key, opts.liveAgents),
          opts.theme,
        )
      : liveEntry
        ? liveGlow(liveEntry.state, liveEntry.agent.color, opts.theme)
        : null
    const avatar = liveEntry
      ? agentAvatarDataUrl(
          liveEntry.agent.emoji?.trim() ||
            initialsAscii(liveEntry.agent.name || liveEntry.agent.id),
          liveEntry.agent.color || opts.theme.accent,
        )
      : undefined
    const background = liveEntry
      ? liveEntry.agent.color || opts.theme.accent
      : boxed
        ? opts.theme.surface2
        : color
    const hiddenChildren =
      opts.collapsed?.has(key) && (opts.childCounts?.get(key) ?? 0) > 0
        ? (opts.childCounts?.get(key) ?? 0)
        : 0
    const node: Node = {
      id: key,
      label:
        truncate(label, depth <= 1 ? 24 : 26) + (hiddenChildren > 0 ? ` [+${hiddenChildren}]` : ''),
      title: tooltip(label, [
        role ? ROLE_LABEL[role] : KIND_LABEL[handle.ref.kind],
        key,
        ...(hiddenChildren > 0 ? [`${hiddenChildren} alt düğüm gizli`] : []),
        ...(liveSessionState === 'running'
          ? ['● çalışıyor']
          : liveSessionState === 'awaiting-workers'
            ? ['◐ worker bekliyor']
            : []),
        ...(liveEntry ? [`oturum: ${liveEntry.session.id}`] : []),
        ...(handle.ref.kind === 'session' ? ['↗ çift tık sohbeti açar'] : []),
      ]),
      shape,
      size: liveEntry ? 18 : size,
      mass: liveEntry ? 0.5 : mass,
      opacity: matches ? 1 : 0.15,
      borderWidth: selected ? 4 : glow ? glow.borderWidth : depth <= 1 ? 2 : 1.5,
      ...(glow ? { shadow: glow.shadow } : {}),
      ...(avatar ? { image: avatar, brokenImage: avatar } : {}),
      color: {
        background,
        border: selected ? opts.theme.text : glow ? glow.border : color,
        highlight: { background, border: opts.theme.text },
        hover: { background, border: opts.theme.text },
      },
      font: {
        color: opts.theme.text,
        size: fontSize,
        strokeWidth: boxed ? 0 : 3,
        strokeColor: opts.theme.bg,
        bold: depth === 0 ? { color: opts.theme.text, size: fontSize, mod: 'bold' } : undefined,
      },
      ...(boxed
        ? {
            shapeProperties: { borderRadius: 6, borderDashes: dashedBox ? [4, 3] : false },
            margin: { top: 5, bottom: 5, left: 9, right: 9 },
          }
        : {}),
      ...(seed ? { x: seed.x, y: seed.y } : {}),
      // The hub is pinned: without an anchor the whole force field drifts and
      // slowly orbits forever. A pinned node still follows a user drag.
      ...(key === ROOT_KEY ? { fixed: { x: true, y: true } } : {}),
    }
    return node
  })

  const pairs = new Set(
    graph.edges.map((edge) => `${refToString(edge.source)} ${refToString(edge.target)}`),
  )
  const edges: Edge[] = graph.edges.map((edge) => {
    const source = refToString(edge.source)
    const target = refToString(edge.target)
    const cyclic = source === target || pairs.has(`${target} ${source}`)
    const targetDepth = opts.layout.depth[target] ?? 2
    const faded = dimmed.has(source) || dimmed.has(target)
    // session -> live avatar: a short, thick tether in the agent's color, no arrow.
    const liveEntry = opts.liveAgents?.get(target)
    const stroke = liveEntry
      ? liveEntry.agent.color || opts.theme.accent
      : cyclic
        ? opts.theme.warning
        : opts.theme.border
    return {
      id: `${source}->${target}`,
      from: source,
      to: target,
      arrows: liveEntry ? undefined : { to: { enabled: true, scaleFactor: 0.45 } },
      dashes: cyclic && !liveEntry ? [6, 4] : false,
      length: liveEntry ? 60 : targetDepth <= 1 ? 320 : targetDepth === 2 ? 150 : 110,
      width: liveEntry ? 2.5 : targetDepth <= 1 ? 1.5 : 1,
      color: {
        color: stroke,
        highlight: opts.theme.textDim,
        opacity: faded ? 0.08 : liveEntry ? 0.9 : 0.7,
      },
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

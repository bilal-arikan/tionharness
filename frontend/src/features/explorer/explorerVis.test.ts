// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import { augmentLive } from './explorerLive'
import { seedLayout } from './explorerSeed'
import {
  displayLabel,
  graphToVis,
  kindColor,
  nodeLabelOfKind,
  nodeRole,
  ROOT_KEY,
  ROOT_REF,
  resolveExplorerTheme,
  type ExplorerTheme,
} from './explorerVis'

const theme: ExplorerTheme = {
  bg: '#000',
  surface: '#111',
  surface2: '#222',
  border: '#333',
  text: '#eee',
  textDim: '#aaa',
  accent: '#0af',
  warning: '#fa0',
}

const sessions: ViewRef = { kind: 'category', id: 'sessions' }
const flows: ViewRef = { kind: 'category', id: 'flows' }
const s1: ViewRef = { kind: 'session', id: 'SES1' }
const s2: ViewRef = { kind: 'session', id: 'SES2' }
const run: ViewRef = { kind: 'flowrun', id: 'RUN1' }

const graph: ViewGraphResult = {
  nodes: [
    { label: 'workspace', ref: ROOT_REF },
    { label: 'Oturumlar', ref: sessions },
    { label: 'Akışlar', ref: flows },
    { label: 'session:SES1 Fix login', ref: s1 },
    { label: 'session:SES2 —', ref: s2 },
    { label: 'run:RUN1 pipeline', ref: run },
  ],
  edges: [
    { source: ROOT_REF, target: sessions },
    { source: ROOT_REF, target: flows },
    { source: sessions, target: s1 },
    { source: sessions, target: s2 },
    { source: flows, target: run },
    { source: s1, target: s2 },
    { source: s2, target: s1 },
    { source: s2, target: s2 },
  ],
}

function vis(overrides: { selectedKey?: string | null; search?: string } = {}) {
  return graphToVis(graph, {
    selectedKey: overrides.selectedKey ?? null,
    search: overrides.search ?? '',
    theme,
    layout: seedLayout(graph, ROOT_KEY),
  })
}

describe('displayLabel', () => {
  it('strips the kind:ID spelling and falls back to the id', () => {
    expect(displayLabel({ label: 'session:SES1 Fix login', ref: s1 })).toBe('Fix login')
    expect(displayLabel({ label: 'session:SES2', ref: s2 })).toBe('SES2')
    expect(
      displayLabel({ label: 'rota:RTA1 plan [running]', ref: { kind: 'trajectory', id: 'RTA1' } }),
    ).toBe('plan [running]')
    expect(displayLabel({ label: 'Oturumlar', ref: sessions })).toBe('Oturumlar')
  })
})

describe('kindColor', () => {
  it('colors board columns by their state and buckets by id', () => {
    expect(kindColor({ kind: 'category', id: 'col:done' })).toBe('#10b981')
    expect(kindColor({ kind: 'category', id: 'col:custom' })).toBe('#64748b')
    expect(kindColor(sessions)).toBe('#0ea5e9')
    expect(kindColor(run)).toBe('#7c3aed')
  })
})

describe('graphToVis', () => {
  it('sizes and weights nodes by depth, keeps ids as ref strings and seeds positions', () => {
    const { nodes } = vis()
    const byId = new Map(nodes.map((n) => [n.id, n]))
    const root = byId.get(ROOT_KEY)!
    const bucket = byId.get('category:sessions')!
    const member = byId.get('session:SES1')!
    expect(root.label).toBe('Workspace')
    expect(root.size).toBeGreaterThan(bucket.size!)
    expect(bucket.size).toBeGreaterThan(member.size!)
    expect(root.mass).toBeGreaterThan(bucket.mass!)
    expect(root.x).toBe(0)
    expect(root.fixed).toEqual({ x: true, y: true })
    expect(member.fixed).toBeUndefined()
    expect(member.shape).toBe('box')
    expect(byId.get('flowrun:RUN1')!.shape).toBe('diamond')
    expect(member.label).toBe('Fix login')
    expect(typeof member.x).toBe('number')
    expect(nodes.every((n) => n.opacity === 1)).toBe(true)
  })

  it('rings the selected node and dims non-matching nodes and their edges on search', () => {
    const { nodes, edges } = vis({ selectedKey: 'session:SES1', search: 'login' })
    const byId = new Map(nodes.map((n) => [n.id, n]))
    expect(byId.get('session:SES1')!.borderWidth).toBe(4)
    expect(byId.get('session:SES1')!.opacity).toBe(1)
    expect(byId.get('session:SES2')!.opacity).toBe(0.15)
    expect(byId.get(ROOT_KEY)!.opacity).toBe(0.15)
    const edgeById = new Map(edges.map((e) => [e.id, e]))
    const faded = edgeById.get('category:sessions->session:SES1')!.color as { opacity: number }
    expect(faded.opacity).toBe(0.08)
    // Search also matches the ref string, so an id query finds its node.
    const byRef = vis({ search: 'RUN1' }).nodes.find((n) => n.id === 'flowrun:RUN1')!
    expect(byRef.opacity).toBe(1)
  })

  it('marks cycles and self-loops as dashed and lengthens hub edges', () => {
    const { edges } = vis()
    const edgeById = new Map(edges.map((e) => [e.id, e]))
    expect(edgeById.get('session:SES1->session:SES2')!.dashes).toEqual([6, 4])
    expect(edgeById.get('session:SES2->session:SES2')!.dashes).toEqual([6, 4])
    expect(edgeById.get('category:sessions->session:SES1')!.dashes).toBe(false)
    expect(edgeById.get('workspace:workspace->category:sessions')!.length).toBeGreaterThan(
      edgeById.get('category:sessions->session:SES1')!.length!,
    )
    expect(edges).toHaveLength(graph.edges.length)
  })

  it('badges a folded node with the number of hidden children', () => {
    const { nodes } = graphToVis(graph, {
      selectedKey: null,
      search: '',
      theme,
      layout: seedLayout(graph, ROOT_KEY),
      collapsed: new Set(['category:sessions', 'session:SES1']),
      childCounts: new Map([['category:sessions', 2]]),
    })
    const byId = new Map(nodes.map((n) => [n.id, n]))
    expect(byId.get('category:sessions')!.label).toBe('Oturumlar [+2]')
    // Folded without children on record: no badge.
    expect(byId.get('session:SES1')!.label).toBe('Fix login')
  })

  it('is deterministic for the same inputs', () => {
    const a = vis({ search: 'x' })
    const b = vis({ search: 'x' })
    const strip = (items: object[]) =>
      items.map((item) => Object.fromEntries(Object.entries(item).filter(([k]) => k !== 'title')))
    expect(strip(a.nodes)).toEqual(strip(b.nodes))
    expect(a.edges).toEqual(b.edges)
  })
})

describe('sub-node roles', () => {
  const board: ViewRef = { kind: 'board', id: 'board' }
  const column: ViewRef = { kind: 'category', id: 'col:done' }
  const card: ViewRef = { kind: 'board', id: 'board', sub: 'T1' }
  const tools: ViewRef = { kind: 'tools', id: 'tools' }
  const group: ViewRef = { kind: 'tools', id: 'tools', sub: 'group:files' }
  const mcp: ViewRef = { kind: 'tools', id: 'tools', sub: 'mcp:M1' }
  const budget: ViewRef = { kind: 'budget', id: 'budget' }
  const provider: ViewRef = { kind: 'budget', id: 'budget', sub: 'provider:anthropic' }
  const roleGraph: ViewGraphResult = {
    nodes: [
      { label: 'workspace', ref: ROOT_REF },
      { label: 'Pano', ref: board },
      { label: 'done (2 kart)', ref: column },
      { label: 'Ship it', ref: card },
      { label: 'Araçlar', ref: tools },
      { label: 'files (11 araç)', ref: group },
      { label: 'MCP linear [aktif]', ref: mcp },
      { label: 'Bütçe', ref: budget },
      { label: 'anthropic · $2.50 · 2 model', ref: provider },
    ],
    edges: [
      { source: ROOT_REF, target: board },
      { source: ROOT_REF, target: tools },
      { source: ROOT_REF, target: budget },
      { source: board, target: column },
      { source: column, target: card },
      { source: tools, target: group },
      { source: tools, target: mcp },
      { source: budget, target: provider },
    ],
  }

  it('groups sessions by kind under the bucket, localized like the kind chips', () => {
    const kind: ViewRef = { kind: 'category', id: 'skind:chat' }
    const other: ViewRef = { kind: 'category', id: 'skind:other' }
    expect(nodeRole(kind)).toBe('session-kind')
    expect(nodeLabelOfKind(kind)).toBe('Oturum türü')
    expect(displayLabel({ label: 'chat (5 oturum)', ref: kind })).toBe('Sohbet (5 oturum)')
    expect(displayLabel({ label: 'other (2 oturum)', ref: other })).toBe('Diğer (2 oturum)')
    expect(kindColor(kind)).toBe('#38bdf8')
  })

  it('classifies roles and localizes their labels', () => {
    expect(nodeRole(column)).toBe('board-column')
    expect(nodeRole(card)).toBe('board-card')
    expect(nodeRole(group)).toBe('tool-group')
    expect(nodeRole(mcp)).toBe('tool-mcp')
    expect(nodeRole(provider)).toBe('budget-provider')
    expect(nodeRole(tools)).toBeNull()
    expect(nodeLabelOfKind(card)).toBe('Kart')
    expect(nodeLabelOfKind(tools)).toBe('Araçlar')
    // Tool groups swap the backend key for the tools screen's Turkish label.
    expect(displayLabel({ label: 'files (11 araç)', ref: group })).toBe('Dosya & Kabuk (11 araç)')
    expect(displayLabel({ label: 'MCP linear [aktif]', ref: mcp })).toBe('MCP linear [aktif]')
  })

  it('gives every role its own shape so columns, cards, groups, servers and providers differ', () => {
    const { nodes } = graphToVis(roleGraph, {
      selectedKey: null,
      search: '',
      theme,
      layout: seedLayout(roleGraph, ROOT_KEY),
    })
    const byId = new Map(nodes.map((n) => [n.id, n]))
    expect(byId.get('category:col:done')!.shape).toBe('square')
    expect(byId.get('category:col:done')!.color).toMatchObject({ background: '#10b981' })
    const cardNode = byId.get('board:board#T1')!
    expect(cardNode.shape).toBe('box')
    expect(cardNode.shapeProperties).toMatchObject({ borderDashes: [4, 3] })
    expect(byId.get('tools:tools#group:files')!.shape).toBe('hexagon')
    expect(byId.get('tools:tools#mcp:M1')!.shape).toBe('triangle')
    expect(byId.get('budget:budget#provider:anthropic')!.shape).toBe('dot')
    expect(byId.get('budget:budget#provider:anthropic')!.color).toMatchObject({
      background: '#16a34a',
    })
  })
})

describe('live layer rendering', () => {
  const live = augmentLive({
    nodes: graph.nodes,
    edges: graph.edges,
    live: [
      {
        session: s1,
        state: 'running',
        agent: { id: 'AG1', name: 'builder', emoji: '🔧', color: '#ff0000' },
      },
      {
        session: s2,
        state: 'awaiting-workers',
        agent: { id: 'AG1', name: 'builder', color: '#ff0000' },
      },
    ],
  })

  it('glows live sessions in the agent color and draws avatar nodes tethered to them', () => {
    const { nodes, edges } = graphToVis(live.graph, {
      selectedKey: null,
      search: '',
      theme,
      layout: seedLayout(live.graph, ROOT_KEY),
      liveState: live.liveState,
      liveAgents: live.liveAgents,
    })
    const byId = new Map(nodes.map((n) => [n.id, n]))
    const running = byId.get('session:SES1')!
    expect(running.shadow).toMatchObject({ enabled: true, color: '#ff0000', size: 28 })
    expect(running.color).toMatchObject({ border: '#ff0000' })
    expect(running.borderWidth).toBe(3)
    const awaiting = byId.get('session:SES2')!
    expect(awaiting.shadow).toMatchObject({ enabled: true, color: theme.warning })
    expect(byId.get('flowrun:RUN1')!.shadow).toBeUndefined()

    const avatar = byId.get('agent:AG1#live:SES1')!
    expect(avatar.shape).toBe('circularImage')
    expect(String(avatar.image)).toMatch(/^data:image\/svg\+xml/)
    expect(avatar.color).toMatchObject({ background: '#ff0000' })
    expect(avatar.label).toBe('builder')
    const tether = edges.find((e) => e.id === 'session:SES1->agent:AG1#live:SES1')!
    expect(tether.arrows).toBeUndefined()
    expect(tether.length).toBe(60)
    expect(tether.color).toMatchObject({ color: '#ff0000' })
  })
})

describe('resolveExplorerTheme', () => {
  it('reads tokens and falls back per missing token', () => {
    document.documentElement.style.setProperty('--color-accent', '#123456')
    const resolved = resolveExplorerTheme()
    expect(resolved.accent).toBe('#123456')
    expect(resolved.warning).toBeTruthy()
    document.documentElement.style.removeProperty('--color-accent')
  })
})

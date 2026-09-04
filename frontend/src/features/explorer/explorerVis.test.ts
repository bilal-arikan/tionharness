// @vitest-environment jsdom

import { describe, expect, it } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import { seedLayout } from './explorerSeed'
import {
  displayLabel,
  graphToVis,
  kindColor,
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

  it('is deterministic for the same inputs', () => {
    const a = vis({ search: 'x' })
    const b = vis({ search: 'x' })
    const strip = (items: object[]) =>
      items.map((item) => Object.fromEntries(Object.entries(item).filter(([k]) => k !== 'title')))
    expect(strip(a.nodes)).toEqual(strip(b.nodes))
    expect(a.edges).toEqual(b.edges)
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

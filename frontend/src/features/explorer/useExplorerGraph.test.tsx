// @vitest-environment jsdom

import { act, useEffect } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const mocks = vi.hoisted(() => ({ viewGraph: vi.fn() }))
vi.mock('@/api', () => ({ api: { viewGraph: mocks.viewGraph } }))

import { useExplorerGraph } from './useExplorerGraph'

const ROOT: ViewRef = { kind: 'workspace', id: 'workspace' }
const sessions: ViewRef = { kind: 'category', id: 'sessions' }
const s1: ViewRef = { kind: 'session', id: 'SES1' }
const graph: ViewGraphResult = {
  nodes: [
    { label: 'workspace', ref: ROOT },
    { label: 'Oturumlar', ref: sessions },
    { label: 'session:SES1 Fix', ref: s1 },
  ],
  edges: [
    { source: ROOT, target: sessions },
    { source: sessions, target: s1 },
  ],
}

type Result = ReturnType<typeof useExplorerGraph>
type Options = Parameters<typeof useExplorerGraph>[0]

const roots = new Set<Root>()
let latest: Result | null = null

function Probe({ options, report }: { options: Options; report: (result: Result) => void }) {
  const result = useExplorerGraph(options)
  useEffect(() => report(result))
  return null
}

async function mount(options: Options) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  roots.add(root)
  const report = (result: Result) => {
    latest = result
  }
  const render = async (next: Options) => {
    await act(async () => root.render(<Probe options={next} report={report} />))
  }
  await render(options)
  return { render, root }
}

beforeEach(() => {
  mocks.viewGraph.mockReset()
  mocks.viewGraph.mockResolvedValue(graph)
  latest = null
})

afterEach(async () => {
  for (const root of [...roots]) {
    await act(async () => root.unmount())
    roots.delete(root)
  }
})

describe('useExplorerGraph', () => {
  it('loads the whole map once, maps it to vis data and selects the root by default', async () => {
    await mount({ search: '' })
    expect(mocks.viewGraph).toHaveBeenCalledTimes(1)
    expect(latest!.ready).toBe(true)
    expect(latest!.nodes.map((n) => n.id)).toEqual([
      'workspace:workspace',
      'category:sessions',
      'session:SES1',
    ])
    expect(latest!.edges).toHaveLength(2)
    expect(latest!.canonicalNodeIds).toHaveLength(3)
    expect(latest!.selectedKey).toBe('workspace:workspace')
    expect(latest!.focusKey).toBeNull()
  })

  it('select focuses the node, bumps the tick on repeat clicks and mirrors to the URL', async () => {
    const onFocus = vi.fn()
    await mount({ search: '', onFocus })
    await act(async () => latest!.selectKey('session:SES1'))
    expect(latest!.selectedRef).toEqual(s1)
    expect(latest!.focusKey).toBe('session:SES1')
    const tick = latest!.focusTick
    expect(onFocus).toHaveBeenLastCalledWith('session:SES1')
    await act(async () => latest!.selectKey('session:SES1'))
    expect(latest!.focusTick).toBe(tick + 1)
    await act(async () => latest!.fallbackToRoot())
    expect(onFocus).toHaveBeenLastCalledWith(null)
    expect(latest!.nodes.find((n) => n.id === 'workspace:workspace')!.borderWidth).toBe(4)
    // An unknown key is ignored rather than selecting a phantom node.
    await act(async () => latest!.selectKey('session:NOPE'))
    expect(latest!.selectedKey).toBe('workspace:workspace')
  })

  it('restores a deep link, follows URL changes and flags a node the map lacks', async () => {
    const { render } = await mount({ search: '', initialFocus: 'session:SES1' })
    expect(latest!.selectedKey).toBe('session:SES1')
    expect(latest!.focusKey).toBe('session:SES1')
    expect(latest!.deepLinkError).toBeUndefined()

    await render({ search: '', initialFocus: 'session:GONE' })
    expect(latest!.selectedKey).toBe('session:GONE')
    expect(latest!.deepLinkError).toContain('session:GONE')

    await render({ search: '', initialFocus: 'garbage' })
    expect(latest!.deepLinkError).toContain('Geçersiz')
    expect(latest!.selectedKey).toBe('workspace:workspace')

    await render({ search: '', initialFocus: null })
    expect(latest!.deepLinkError).toBeUndefined()
  })

  it('reports a failed load and recovers on refresh', async () => {
    const onError = vi.fn()
    mocks.viewGraph.mockRejectedValueOnce(new Error('boom'))
    await mount({ search: '', onError })
    expect(latest!.error).toBe('boom')
    expect(latest!.ready).toBe(false)
    expect(onError).toHaveBeenCalledWith('boom')
    await act(async () => latest!.refresh())
    expect(latest!.error).toBeUndefined()
    expect(latest!.ready).toBe(true)
  })

  it('adds the live layer, filters it and projects an avatar as its agent', async () => {
    mocks.viewGraph.mockResolvedValue({
      ...graph,
      live: [{ session: s1, state: 'running', agent: { id: 'AG1', name: 'builder' } }],
      meta: { 'session:SES1': { kind: 'chat', agentId: 'AG1' } },
    })
    const { render } = await mount({ search: '' })
    expect(latest!.liveCount).toBe(1)
    expect(latest!.nodes.map((n) => n.id)).toContain('agent:AG1#live:SES1')
    expect(latest!.canonicalNodeIds).toContain('agent:AG1#live:SES1')
    expect(latest!.buckets.map((b) => b.key)).toEqual(['category:sessions'])
    expect(latest!.facets.kinds).toEqual(['chat'])
    await act(async () => latest!.selectKey('agent:AG1#live:SES1'))
    expect(latest!.selectedKey).toBe('agent:AG1#live:SES1')
    expect(latest!.panelRef).toEqual({ kind: 'agent', id: 'AG1' })
    await render({
      search: '',
      filter: { hiddenBuckets: [], liveOnly: false, kinds: ['task'], agentIds: [], tags: [] },
    })
    expect(latest!.visibleGraph!.nodes.map((h) => h.ref.id)).not.toContain('SES1')
    expect(latest!.nodes.map((n) => n.id)).not.toContain('agent:AG1#live:SES1')
    // Persistence GC still sees every node, filtered or not.
    expect(latest!.canonicalNodeIds).toContain('session:SES1')
  })

  it('dims non-matching nodes when the search changes', async () => {
    const { render } = await mount({ search: '' })
    await render({ search: 'fix' })
    const byId = new Map(latest!.nodes.map((n) => [n.id, n.opacity]))
    expect(byId.get('session:SES1')).toBe(1)
    expect(byId.get('category:sessions')).toBe(0.15)
  })
})

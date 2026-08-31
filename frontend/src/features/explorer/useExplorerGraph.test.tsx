// @vitest-environment jsdom

import { act, useEffect } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ViewNeighborhoodResult, ViewRef } from '@/types'
import { useExplorerGraph } from './useExplorerGraph'

const mocks = vi.hoisted(() => ({ viewNeighborhood: vi.fn() }))
vi.mock('@/api', () => ({ api: { viewNeighborhood: mocks.viewNeighborhood } }))

type GraphState = ReturnType<typeof useExplorerGraph>
let latest: GraphState
const roots: ReturnType<typeof createRoot>[] = []
const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

function neighborhood(ref: ViewRef): ViewNeighborhoodResult {
  return {
    focus: { ref, label: ref.id },
    parents: [],
    children: [],
    hiddenParentCount: 0,
    hiddenChildCount: 0,
  }
}

function Harness({
  initialFocus,
  onFocus,
  onError,
}: {
  initialFocus?: string | null
  onFocus?: (ref: string | null) => void
  onError?: (message: string) => void
}) {
  const state = useExplorerGraph({ search: '', initialFocus, onFocus, onError })
  useEffect(() => {
    latest = state
  })
  return null
}

function renderHarness(
  initialFocus?: string | null,
  onFocus?: (ref: string | null) => void,
  onError?: (message: string) => void,
) {
  const container = document.createElement('div')
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(<Harness initialFocus={initialFocus} onFocus={onFocus} onError={onError} />),
  )
  return root
}

beforeEach(() => {
  mocks.viewNeighborhood.mockImplementation(async (ref: ViewRef) => neighborhood(ref))
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  vi.clearAllMocks()
})

describe('useExplorerGraph focus deep-links', () => {
  it('uses workspace root when the URL has no focus', async () => {
    renderHarness()
    await act(async () => {})

    expect(mocks.viewNeighborhood).toHaveBeenCalledWith(
      { kind: 'workspace', id: 'workspace' },
      expect.any(AbortSignal),
    )
  })

  it('restores a valid focus and reports focus changes for URL sync', async () => {
    const onFocus = vi.fn()
    renderHarness('agent:AG1', onFocus)
    await act(async () => {})
    expect(latest.focusRef).toEqual({ kind: 'agent', id: 'AG1' })

    await act(async () => latest.focus({ kind: 'skill', id: 'reviewer' }))
    expect(onFocus).toHaveBeenCalledWith('skill:reviewer')
  })

  it('restores the complete root-to-focus lineage for a deep-link', async () => {
    const root = { kind: 'workspace' as const, id: 'workspace' }
    const category = { kind: 'category' as const, id: 'agents' }
    const agent = { kind: 'agent' as const, id: 'AG1' }
    mocks.viewNeighborhood.mockImplementation(async (ref: ViewRef) => {
      const data = neighborhood(ref)
      if (ref.id === 'SES1') data.parents = [{ ref: agent, label: 'Agent One' }]
      if (ref.id === 'AG1') data.parents = [{ ref: category, label: 'Agents' }]
      if (ref.id === 'agents') data.parents = [{ ref: root, label: 'workspace' }]
      return data
    })

    renderHarness('session:SES1')
    await act(async () => {})

    expect(latest.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining([
        'workspace:workspace',
        'category:agents',
        'agent:AG1',
        'session:SES1',
      ]),
    )
    expect(latest.edges.map((edge) => edge.id)).toEqual(
      expect.arrayContaining([
        'workspace:workspace->category:agents',
        'category:agents->agent:AG1',
        'agent:AG1->session:SES1',
      ]),
    )
  })

  it('shows malformed refs and falls back to root without hiding the error', async () => {
    const onFocus = vi.fn()
    renderHarness('galaxy:x', onFocus)
    await act(async () => {})

    expect(latest.deepLinkError).toContain('galaxy:x')
    expect(latest.focusRef).toEqual({ kind: 'workspace', id: 'workspace' })
    await act(async () => latest.fallbackToRoot())
    expect(onFocus).toHaveBeenCalledWith(null)
  })

  it('ignores stale responses after rapid focus changes', async () => {
    const resolvers = new Map<string, (value: ViewNeighborhoodResult) => void>()
    mocks.viewNeighborhood.mockImplementation(
      (ref: ViewRef) =>
        new Promise<ViewNeighborhoodResult>((resolve) => resolvers.set(ref.id, resolve)),
    )
    renderHarness()
    act(() => latest.focus({ kind: 'agent', id: 'A' }))
    act(() => latest.focus({ kind: 'agent', id: 'B' }))

    await act(async () => resolvers.get('A')?.(neighborhood({ kind: 'agent', id: 'A' })))
    expect(latest.focusRef).toEqual({ kind: 'agent', id: 'B' })
    expect(latest.nodes).toEqual([])

    await act(async () => resolvers.get('B')?.(neighborhood({ kind: 'agent', id: 'B' })))
    expect(latest.nodes.some((node) => node.id === 'agent:B')).toBe(true)
  })

  it('stops cyclic ancestry and ignores an obsolete deep-link resolver', async () => {
    const onError = vi.fn()
    const cyclicA = { kind: 'agent' as const, id: 'A' }
    const cyclicB = { kind: 'agent' as const, id: 'B' }
    mocks.viewNeighborhood.mockImplementation(async (ref: ViewRef) => {
      const data = neighborhood(ref)
      if (ref.id === 'A') data.parents = [{ ref: cyclicB, label: 'B' }]
      if (ref.id === 'B') data.parents = [{ ref: cyclicA, label: 'A' }]
      return data
    })
    const root = renderHarness('agent:A', undefined, onError)
    await act(async () => {})

    expect(latest.deepLinkError).toBe('Odak köke bağlanamadı: agent:A')
    expect(onError).toHaveBeenCalledWith('Odak köke bağlanamadı: agent:A')
    expect(mocks.viewNeighborhood.mock.calls.length).toBeLessThanOrEqual(3)

    const staleResolvers: Array<(value: ViewNeighborhoodResult) => void> = []
    mocks.viewNeighborhood.mockImplementation((ref: ViewRef) =>
      ref.id === 'A'
        ? new Promise<ViewNeighborhoodResult>((resolve) => staleResolvers.push(resolve))
        : Promise.resolve(neighborhood(ref)),
    )
    act(() => root.render(<Harness initialFocus="workspace:workspace" onError={onError} />))
    await act(async () => {})
    act(() => root.render(<Harness initialFocus="agent:A" onError={onError} />))
    await act(async () => {})
    act(() => root.render(<Harness initialFocus="workspace:workspace" onError={onError} />))
    await act(async () => {})
    await act(async () => {
      for (const resolve of staleResolvers) resolve(neighborhood(cyclicA))
    })

    expect(latest.focusRef).toEqual({ kind: 'workspace', id: 'workspace' })
    expect(latest.nodes.some((node) => node.id === 'agent:A')).toBe(false)
    expect(latest.deepLinkError).toBeUndefined()
  })

  it('keeps ancestors through child navigation and trims them when returning', async () => {
    const root = { kind: 'workspace' as const, id: 'workspace' }
    const branch = { kind: 'category' as const, id: 'agents' }
    const leaf = { kind: 'agent' as const, id: 'A' }
    mocks.viewNeighborhood.mockImplementation(async (ref: ViewRef) => {
      const data = neighborhood(ref)
      if (ref.id === 'workspace') data.children = [{ ref: branch, label: 'Agents' }]
      if (ref.id === 'agents') {
        data.parents = [{ ref: root, label: 'workspace' }]
        data.children = [{ ref: leaf, label: 'Agent A' }]
      }
      if (ref.id === 'A') data.parents = [{ ref: branch, label: 'Agents' }]
      return data
    })
    renderHarness()
    await act(async () => {})
    await act(async () => latest.focus(branch))
    await act(async () => {})
    await act(async () => latest.focus(leaf))
    await act(async () => {})

    expect(latest.nodes.map((node) => node.id)).toEqual(
      expect.arrayContaining(['workspace:workspace', 'category:agents', 'agent:A']),
    )
    await act(async () => latest.focus(branch))
    await act(async () => {})
    expect(latest.nodes.find((node) => node.id === 'agent:A')?.data.depth).toBe(2)
    expect(latest.nodes.some((node) => node.id === 'workspace:workspace')).toBe(true)
  })

  it('uses current lineage during rapid focus transitions', async () => {
    const branch = { kind: 'category' as const, id: 'agents' }
    const leaf = { kind: 'agent' as const, id: 'A' }
    mocks.viewNeighborhood.mockImplementation(async (ref: ViewRef) => {
      const data = neighborhood(ref)
      if (ref.id === 'workspace') data.children = [{ ref: branch, label: 'Agents' }]
      if (ref.id === 'agents') data.children = [{ ref: leaf, label: 'Agent A' }]
      return data
    })
    renderHarness()
    await act(async () => {})

    act(() => latest.focus(branch))
    await act(async () => {})
    act(() => {
      latest.focus(leaf)
      latest.focus(branch)
    })
    await act(async () => {})

    expect(latest.focusRef).toEqual(branch)
    expect(latest.nodes.some((node) => node.id === 'workspace:workspace')).toBe(true)
    expect(latest.nodes.some((node) => node.id === 'agent:A')).toBe(true)
  })
})

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
}: {
  initialFocus?: string | null
  onFocus?: (ref: string | null) => void
}) {
  const state = useExplorerGraph({ search: '', initialFocus, onFocus })
  useEffect(() => {
    latest = state
  })
  return null
}

function renderHarness(initialFocus?: string | null, onFocus?: (ref: string | null) => void) {
  const container = document.createElement('div')
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<Harness initialFocus={initialFocus} onFocus={onFocus} />))
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
})

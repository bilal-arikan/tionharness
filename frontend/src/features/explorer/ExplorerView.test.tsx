// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ViewRef } from '@/types'
import { ExplorerView } from './ExplorerView'

const mocks = vi.hoisted(() => ({
  graphState: {} as Record<string, unknown>,
  refreshFocused: vi.fn(),
}))

vi.mock('@/shared/hooks/useRefreshTrigger', () => ({ useRefreshTrigger: () => 0 }))
vi.mock('@/features/view/ViewPanel', () => ({ ViewPanel: () => <div>detail panel</div> }))
vi.mock('./ExplorerGraph', () => ({ ExplorerGraph: () => <div>graph canvas</div> }))
vi.mock('./useExplorerGraph', () => ({ useExplorerGraph: () => mocks.graphState }))

const rootRef: ViewRef = { kind: 'workspace', id: 'root' }
const roots: ReturnType<typeof createRoot>[] = []
const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

function renderView() {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<ExplorerView onError={() => {}} />))
  return container
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.graphState = {
    nodes: [],
    edges: [],
    select: vi.fn(),
    focus: vi.fn(),
    selectedRef: rootRef,
    focusLoading: false,
    focusError: undefined,
    refreshFocused: mocks.refreshFocused,
  }
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('ExplorerView focus request status', () => {
  it('shows loading status over the graph', () => {
    mocks.graphState.focusLoading = true

    const container = renderView()

    expect(container.querySelector('[role="status"]')?.textContent).toContain(
      'Odak çevresi yükleniyor',
    )
    expect(container.textContent).toContain('graph canvas')
  })

  it('shows focus errors instead of presenting an empty graph as a leaf', () => {
    mocks.graphState.focusError = 'network unavailable'

    const container = renderView()

    expect(container.querySelector('[role="alert"]')?.textContent).toContain(
      'Odak çevresi yüklenemedi: network unavailable',
    )
  })

  it('retries the active focus request', () => {
    mocks.graphState.focusError = 'network unavailable'
    const container = renderView()
    const retry = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.includes('Tekrar dene'),
    )
    mocks.refreshFocused.mockClear()

    act(() => retry?.click())

    expect(mocks.refreshFocused).toHaveBeenCalledOnce()
  })
})

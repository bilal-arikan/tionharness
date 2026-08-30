// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ViewRef } from '@/types'
import { ExplorerView } from './ExplorerView'

const mocks = vi.hoisted(() => ({
  graphState: {} as Record<string, unknown>,
  refreshFocused: vi.fn(),
  graphProps: {} as Record<string, unknown>,
  viewPanelProps: {} as Record<string, unknown>,
}))

vi.mock('@/shared/hooks/useRefreshTrigger', () => ({ useRefreshTrigger: () => 0 }))
vi.mock('@/features/view/ViewPanel', () => ({
  ViewPanel: (props: Record<string, unknown>) => {
    mocks.viewPanelProps = props
    return <div>detail panel</div>
  },
}))
vi.mock('./ExplorerGraph', () => ({
  ExplorerGraph: (props: Record<string, unknown>) => {
    mocks.graphProps = props
    return <div>graph canvas</div>
  },
}))
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
    deepLinkError: undefined,
    fallbackToRoot: vi.fn(),
    refreshFocused: mocks.refreshFocused,
  }
  vi.stubGlobal(
    'matchMedia',
    vi.fn(() => ({
      matches: false,
      media: '',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  vi.unstubAllGlobals()
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

  it('offers a workspace-root fallback for an invalid deep-link', () => {
    mocks.graphState.deepLinkError = 'Geçersiz odak bağlantısı: galaxy:x'
    const container = renderView()

    act(() => {
      ;[...container.querySelectorAll('button')]
        .find((button) => button.textContent?.includes('Workspace köküne dön'))
        ?.click()
    })

    expect(mocks.graphState.fallbackToRoot).toHaveBeenCalledOnce()
  })

  it('keeps search selection separate from focus and focuses on double click', () => {
    const ref: ViewRef = { kind: 'agent', id: 'AG1' }
    mocks.graphState.nodes = [{ id: 'agent:AG1', data: { ref, label: 'Builder', selected: false } }]
    const container = renderView()
    const input = container.querySelector('input')!
    act(() => {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
      setter.call(input, 'build')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const result = container.querySelector('[role="option"]') as HTMLButtonElement

    act(() => result.click())
    expect(mocks.graphState.select).toHaveBeenCalledWith(ref)
    expect(mocks.graphState.focus).not.toHaveBeenCalled()

    act(() => result.dispatchEvent(new MouseEvent('dblclick', { bubbles: true })))
    expect(mocks.graphState.focus).toHaveBeenCalledWith(ref)
  })

  it('shows the selected node in the existing detail panel', () => {
    const selectedRef: ViewRef = { kind: 'skill', id: 'reviewer' }
    mocks.graphState.selectedRef = selectedRef

    renderView()

    expect(mocks.viewPanelProps.target).toEqual(selectedRef)
  })

  it('opens every overflow handle in a selectable list', () => {
    const container = renderView()
    const handles = [
      { label: 'Agent A', ref: { kind: 'agent' as const, id: 'A' } },
      { label: 'Agent B', ref: { kind: 'agent' as const, id: 'B' } },
    ]

    act(() => {
      ;(
        mocks.graphProps.onOverflowClick as (
          side: 'parents' | 'children',
          items: typeof handles,
        ) => void
      )('parents', handles)
    })

    expect(container.querySelector('[role="dialog"]')?.textContent).toContain(
      'Kalan üst bağlantılar',
    )
    expect(container.textContent).toContain('Agent A')
    expect(container.textContent).toContain('Agent B')
  })

  it('announces focus changes with node name and child count', () => {
    mocks.graphState.nodes = [
      {
        id: 'agent:A',
        data: { ref: { kind: 'agent', id: 'A' }, label: 'Ajan A', focus: true, childCount: 3 },
      },
    ]

    const container = renderView()

    expect(container.querySelector('[aria-live="polite"]')?.textContent).toContain(
      'Harita odağı Ajan A. 3 alt bağlantı.',
    )
  })

  it('uses a detail drawer at the narrow breakpoint and returns focus on Escape', () => {
    vi.mocked(window.matchMedia).mockReturnValue({
      matches: true,
      media: '(max-width: 1023px)',
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })
    vi.useFakeTimers()
    const container = renderView()
    const trigger = document.createElement('button')
    document.body.appendChild(trigger)
    trigger.focus()

    act(() => (mocks.graphProps.onNodeClick as (ref: ViewRef) => void)(rootRef))
    act(() => vi.advanceTimersByTime(250))
    expect(container.querySelector('[aria-label="Seçili düğüm detayı"]')).not.toBeNull()
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Düğüm detayını kapat')

    act(() => document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' })))
    act(() => vi.runAllTimers())
    expect(container.querySelector('[aria-label="Seçili düğüm detayı"]')).toBeNull()
    expect(document.activeElement).toBe(trigger)
    vi.useRealTimers()
  })
})

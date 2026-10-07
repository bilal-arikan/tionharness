// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ViewGraphResult, ViewRef } from '@/types'
import { i18next } from '@/i18n'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

interface GraphProps {
  workspaceId: string
  layoutId?: string
  nodes: { id: string }[]
  canonicalReady: boolean
  focusNodeId?: string | null
  focusTick?: number
  onSelect?: (id: string | null) => void
  onNodeDoubleClick?: (id: string) => void
  mode?: string
  settle?: boolean
  density?: number
}

const mocks = vi.hoisted(() => ({
  viewGraph: vi.fn(),
  graphProps: [] as GraphProps[],
  viewPanelTargets: [] as string[],
}))

vi.mock('@/api', () => ({ api: { viewGraph: mocks.viewGraph } }))
vi.mock('@/shared/hooks/useRefreshTrigger', () => ({ useRefreshTrigger: () => 0 }))
vi.mock('@/shared/hooks/useMediaQuery', () => ({ useIsMobile: () => false }))
vi.mock('@/features/network/VisNetworkGraph', () => ({
  VisNetworkGraph: (props: GraphProps) => {
    mocks.graphProps.push(props)
    return <div data-testid="network" data-layout={props.layoutId} />
  },
}))
vi.mock('@/features/view/ViewPanel', () => ({
  ViewPanel: ({ target }: { target: ViewRef }) => {
    const key = `${target.kind}:${target.id}`
    mocks.viewPanelTargets.push(key)
    return <div data-testid="view-panel">{key}</div>
  },
}))

import { ExplorerView } from './ExplorerView'

const ROOT: ViewRef = { kind: 'workspace', id: 'workspace' }
const sessions: ViewRef = { kind: 'category', id: 'sessions' }
const s1: ViewRef = { kind: 'session', id: 'SES1' }
const graph: ViewGraphResult = {
  nodes: [
    { label: 'workspace', ref: ROOT },
    { label: 'Oturumlar', ref: sessions },
    { label: 'session:SES1 Fix login', ref: s1 },
  ],
  edges: [
    { source: ROOT, target: sessions },
    { source: sessions, target: s1 },
  ],
}

const roots = new Set<Root>()

async function mount(
  props: Partial<Parameters<typeof ExplorerView>[0]> = {},
): Promise<{ host: HTMLElement; root: Root }> {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  roots.add(root)
  await act(async () =>
    root.render(<ExplorerView workspaceId="ws1" onError={() => {}} {...props} />),
  )
  return { host, root }
}

function latestGraphProps(): GraphProps {
  const props = mocks.graphProps.at(-1)
  if (!props) throw new Error('VisNetworkGraph not rendered')
  return props
}

beforeEach(async () => {
  await i18next.changeLanguage('tr')
  localStorage.clear()
  mocks.viewGraph.mockReset()
  mocks.viewGraph.mockResolvedValue(graph)
  mocks.graphProps.length = 0
  mocks.viewPanelTargets.length = 0
  window.matchMedia = vi
    .fn()
    .mockReturnValue({ matches: false }) as unknown as typeof window.matchMedia
})

afterEach(async () => {
  for (const root of [...roots]) {
    await act(async () => root.unmount())
    roots.delete(root)
  }
  document.body.innerHTML = ''
})

describe('ExplorerView', () => {
  it('remembers the density slider across remounts', async () => {
    const first = await mount()
    // The slider lives in the folded filter panel.
    const toggle = first.host.querySelector<HTMLButtonElement>(
      '[data-testid="explorer-filters-toggle"]',
    )!
    await act(async () => toggle.click())
    const slider = first.host.querySelector<HTMLInputElement>('input[type="range"]')!
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(slider, '1.6')
      slider.dispatchEvent(new Event('input', { bubbles: true }))
    })
    expect(latestGraphProps().density).toBe(1.6)
    await act(async () => first.root.unmount())
    roots.delete(first.root)
    await mount()
    expect(latestGraphProps().density).toBe(1.6)
  })

  it('folds the facets behind one toggle and remembers the live filter', async () => {
    mocks.viewGraph.mockResolvedValue({
      ...graph,
      live: [{ session: s1, state: 'running', agent: { id: 'AG1', name: 'builder' } }],
      meta: { 'session:SES1': { kind: 'chat', agentId: 'AG1', tags: ['t'] } },
    })
    const first = await mount()
    const group = first.host.querySelector('[aria-label="Harita filtreleri"]')!
    // Folded by default: the time window and the counters show, the facets do not.
    expect(group.textContent).toContain('24 sa')
    expect(group.textContent).not.toContain('#t')
    await act(async () =>
      group.querySelector<HTMLButtonElement>('[data-testid="explorer-filters-toggle"]')!.click(),
    )
    expect(group.textContent).toContain('Oturumlar')
    expect(group.textContent).toContain('#t')
    // The running counter is the live-only toggle.
    const liveChip = [...group.querySelectorAll('button')].find((b) =>
      b.textContent?.startsWith('çalışan'),
    )!
    expect(liveChip.textContent).toContain('çalışan · 1')
    await act(async () => liveChip.click())
    expect(liveChip.getAttribute('aria-pressed')).toBe('true')
    expect(first.host.textContent).toContain('1 filtre · temizle')
    await act(async () => first.root.unmount())
    roots.delete(first.root)
    const second = await mount()
    const again = [...second.host.querySelectorAll('button')].find((b) =>
      b.textContent?.startsWith('çalışan'),
    )!
    expect(again.getAttribute('aria-pressed')).toBe('true')
  })

  it('shows the status strip and narrows the map from a counter', async () => {
    const s2: ViewRef = { kind: 'session', id: 'SES2' }
    mocks.viewGraph.mockResolvedValue({
      ...graph,
      nodes: [...graph.nodes, { label: 'session:SES2 Calm', ref: s2 }],
      edges: [...graph.edges, { source: sessions, target: s2 }],
      attention: { 'session:SES1': { level: 'danger', reasons: ['stuck'] } },
      status: {
        at: 1,
        running: 0,
        waiting: 0,
        stuck: 1,
        failedCards: 0,
        failedRuns: 0,
        stale: 0,
        notesToday: 3,
      },
    })
    const { host } = await mount()
    const strip = host.querySelector('[aria-label="Durum şeridi"]')!
    expect(strip.textContent).toContain('takılan · 1')
    // Zero counters are not drawn; notes and digest age ride in the info tooltip.
    expect(strip.textContent).not.toContain('seni bekleyen')
    const info = strip.querySelector('[role="img"]')!
    expect(info.getAttribute('title')).toContain('bugün 3 not')
    expect(info.getAttribute('title')).toContain('henüz özet yok')
    const stuck = [...strip.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('takılan'),
    )!
    expect(latestGraphProps().nodes.map((n) => n.id)).toContain('session:SES2')
    await act(async () => stuck.click())
    expect(stuck.getAttribute('aria-pressed')).toBe('true')
    const ids = latestGraphProps().nodes.map((n) => n.id)
    expect(ids).toContain('session:SES1')
    expect(ids).not.toContain('session:SES2')
    expect(host.textContent).toContain('1 filtre · temizle')
    // Clearing drops the facet but keeps the time window.
    const clear = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('temizle'),
    )!
    await act(async () => clear.click())
    expect(latestGraphProps().nodes.map((n) => n.id)).toContain('session:SES2')
    const day = [...host.querySelectorAll('button')].find((b) => b.textContent === '24 sa')!
    expect(day.getAttribute('aria-pressed')).toBe('true')
  })

  it('folds and unfolds the selected node from the side panel and remembers it', async () => {
    const first = await mount()
    // The root has children: the toggle is offered and hides the subtree.
    const hide = [...first.host.querySelectorAll('button')].find((b) =>
      b.textContent?.startsWith('Alt düğümleri gizle'),
    )!
    expect(hide.textContent).toContain('· 1')
    await act(async () => hide.click())
    expect(latestGraphProps().nodes.map((n) => n.id)).toEqual(['workspace:workspace'])
    const show = [...first.host.querySelectorAll('button')].find((b) =>
      b.textContent?.startsWith('Alt düğümleri göster'),
    )!
    expect(show.getAttribute('aria-pressed')).toBe('true')
    // A leaf offers no toggle.
    await act(async () => latestGraphProps().onSelect?.('session:SES1'))
    expect(
      [...first.host.querySelectorAll('button')].some((b) =>
        b.textContent?.includes('Alt düğümleri'),
      ),
    ).toBe(false)
    await act(async () => first.root.unmount())
    roots.delete(first.root)
    // Persisted per workspace.
    await mount()
    expect(latestGraphProps().nodes.map((n) => n.id)).toEqual(['workspace:workspace'])
    await mount({ workspaceId: 'ws2' })
    expect(latestGraphProps().nodes).toHaveLength(3)
  })

  it('renders the whole map on its own layout scope with the root selected', async () => {
    const { host } = await mount()
    const props = latestGraphProps()
    expect(props.layoutId).toBe('explorer:ws1')
    expect(props.workspaceId).toBe('ws1')
    expect(props.mode).toBe('tree')
    // Continuous physics: the map never switches itself off (user choice, 2026-09-05).
    expect(props.settle).toBeUndefined()
    expect(props.canonicalReady).toBe(true)
    expect(props.nodes.map((n) => n.id)).toContain('session:SES1')
    expect(host.textContent).toContain('3/3 düğüm')
    expect(host.querySelector('[data-testid="view-panel"]')!.textContent).toBe(
      'workspace:workspace',
    )
  })

  it('a click selects, focuses the camera, updates the panel and the URL', async () => {
    const onFocusNode = vi.fn()
    const onOpenTarget = vi.fn()
    const { host } = await mount({ onFocusNode, onOpenTarget })
    // The root already offers its screen.
    expect(host.textContent).toContain('Workspace ekranında aç')
    await act(async () => latestGraphProps().onSelect?.('session:SES1'))
    const props = latestGraphProps()
    expect(props.focusNodeId).toBe('session:SES1')
    expect(host.querySelector('[data-testid="view-panel"]')!.textContent).toBe('session:SES1')
    expect(onFocusNode).toHaveBeenLastCalledWith('session:SES1')
    expect(host.textContent).toContain('Sohbet ekranında aç · SES1')
    const open = [...host.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('Sohbet ekranında aç'),
    )!
    await act(async () => open.click())
    expect(onOpenTarget).toHaveBeenCalledWith({ view: 'chat', id: 'SES1', label: 'Sohbet' })
    // Clicking empty canvas (deselect) keeps the selection.
    await act(async () => latestGraphProps().onSelect?.(null))
    expect(host.querySelector('[data-testid="view-panel"]')!.textContent).toBe('session:SES1')
  })

  it('double click on a session opens its transcript; other kinds are ignored', async () => {
    const onOpenSession = vi.fn()
    await mount({ onOpenSession })
    await act(async () => latestGraphProps().onNodeDoubleClick?.('session:SES1'))
    expect(onOpenSession).toHaveBeenCalledWith('SES1')
    await act(async () => latestGraphProps().onNodeDoubleClick?.('category:sessions'))
    expect(onOpenSession).toHaveBeenCalledTimes(1)
  })

  it('search lists matches and picking one selects it', async () => {
    const { host } = await mount()
    const input = host.querySelector<HTMLInputElement>('input[aria-label="Haritada ara"]')!
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!
    await act(async () => {
      setter.call(input, 'login')
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    const options = host.querySelectorAll('[role="option"]')
    expect(options).toHaveLength(1)
    expect(options[0].textContent).toContain('Fix login')
    await act(async () => (options[0] as HTMLButtonElement).click())
    expect(latestGraphProps().focusNodeId).toBe('session:SES1')
    // The canvas gets the dimmed data too.
    const dimmed = latestGraphProps().nodes as { id: string; opacity?: number }[]
    expect(dimmed.find((n) => n.id === 'category:sessions')!.opacity).toBe(0.15)
  })

  it('restores a deep link and offers the root when the node is missing', async () => {
    const { host } = await mount({ focusNode: 'session:SES1' })
    expect(latestGraphProps().focusNodeId).toBe('session:SES1')
    expect(host.querySelector('[role="alert"]')).toBeNull()

    const onFocusNode = vi.fn()
    const second = await mount({ focusNode: 'session:GONE', onFocusNode })
    const alert = second.host.querySelector('[role="alert"]')!
    expect(alert.textContent).toContain('session:GONE')
    await act(async () => (alert.querySelector('button') as HTMLButtonElement).click())
    expect(onFocusNode).toHaveBeenLastCalledWith(null)
    expect(second.host.querySelector('[role="alert"]')).toBeNull()
  })

  it('shows a retryable error when the map fails to load', async () => {
    mocks.viewGraph.mockRejectedValueOnce(new Error('offline'))
    const onError = vi.fn()
    const { host } = await mount({ onError })
    const alert = host.querySelector('[role="alert"]')!
    expect(alert.textContent).toContain('offline')
    expect(onError).toHaveBeenCalledWith('offline')
    await act(async () => (alert.querySelector('button') as HTMLButtonElement).click())
    expect(host.querySelector('[role="alert"]')).toBeNull()
    expect(latestGraphProps().canonicalReady).toBe(true)
  })

  it('opens the detail drawer on narrow screens after a selection and closes on Escape', async () => {
    window.matchMedia = vi
      .fn()
      .mockReturnValue({ matches: true }) as unknown as typeof window.matchMedia
    const { host } = await mount()
    await act(async () => latestGraphProps().onSelect?.('category:sessions'))
    const dialog = host.querySelector('[role="dialog"]')!
    expect(dialog).not.toBeNull()
    expect(dialog.textContent).toContain('category:sessions')
    await act(async () => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    })
    expect(host.querySelector('[role="dialog"]')).toBeNull()
  })
})

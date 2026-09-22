// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { SearchIndexStatus } from '@/types'
import { SearchIndexPanel } from './SearchIndexPanel'

const listSearchIndexes = vi.fn()
const refreshSearchIndex = vi.fn()
const dropSearchIndex = vi.fn()

vi.mock('@/api', () => ({
  api: {
    listSearchIndexes: () => listSearchIndexes(),
    refreshSearchIndex: (tool: string, root: string, rebuild: boolean) =>
      refreshSearchIndex(tool, root, rebuild),
    dropSearchIndex: (tool: string, root: string, confirmRoot: string) =>
      dropSearchIndex(tool, root, confirmRoot),
  },
}))

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []

const row = (over: Partial<SearchIndexStatus> = {}): SearchIndexStatus => ({
  tool: 'zg',
  root: 'C:/repo',
  phase: 'ready',
  usable: true,
  updatedAt: '2026-09-22T10:30:00Z',
  ...over,
})

async function renderPanel(onError = vi.fn()) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  await act(async () => {
    root.render(<SearchIndexPanel onError={onError} />)
  })
  return { container, onError }
}

const q = (c: HTMLElement, testid: string) =>
  c.querySelector<HTMLElement>(`[data-testid="${testid}"]`)

// jest-dom is not installed, so disabled-ness is read off the element itself.
const disabled = (el: HTMLElement | null) => (el as HTMLButtonElement | null)?.disabled

// Type into a CONTROLLED input. Assigning `.value` directly is not enough:
// React installs its own value setter on the element and would read the stale
// one back, leaving state empty. The native setter is what React's onChange
// actually observes.
async function type(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set
  await act(async () => {
    setter?.call(input, value)
    input.dispatchEvent(new Event('input', { bubbles: true }))
  })
}

afterEach(() => {
  act(() => roots.splice(0).forEach((root) => root.unmount()))
  document.body.innerHTML = ''
  vi.clearAllMocks()
  vi.useRealTimers()
})

describe('SearchIndexPanel', () => {
  it('lists one row per index with its phase and root', async () => {
    listSearchIndexes.mockResolvedValue([row(), row({ root: 'C:/other', phase: 'stale' })])

    const { container } = await renderPanel()

    const rows = container.querySelectorAll('[data-testid="search-index-row"]')
    expect(rows).toHaveLength(2)
    expect(rows[0].getAttribute('data-phase')).toBe('ready')
    expect(rows[1].getAttribute('data-root')).toBe('C:/other')
  })

  it('shows the reason a failed index failed rather than hiding it', async () => {
    listSearchIndexes.mockResolvedValue([
      row({ phase: 'failed', usable: false, error: 'zg exited 1' }),
    ])

    const { container } = await renderPanel()

    expect(q(container, 'search-index-error')?.textContent).toContain('zg exited 1')
  })

  it('disables the actions while a run is in flight', async () => {
    listSearchIndexes.mockResolvedValue([row({ phase: 'indexing' })])

    const { container } = await renderPanel()

    // A second run would be refused with 409, so the buttons must not invite it.
    expect(disabled(q(container, 'search-index-refresh'))).toBe(true)
    expect(disabled(q(container, 'search-index-rebuild'))).toBe(true)
    expect(disabled(q(container, 'search-index-drop-open'))).toBe(true)
  })

  it('refetches while something is indexing and stops once it settles', async () => {
    vi.useFakeTimers()
    listSearchIndexes.mockResolvedValue([row({ phase: 'indexing' })])
    const { container } = await renderPanel()
    expect(listSearchIndexes).toHaveBeenCalledTimes(1)

    // The run finishes; the next poll picks that up.
    listSearchIndexes.mockResolvedValue([row({ phase: 'ready' })])
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100)
    })
    expect(listSearchIndexes).toHaveBeenCalledTimes(2)
    expect(q(container, 'search-index-row')?.getAttribute('data-phase')).toBe('ready')

    // Nothing is indexing any more, so polling must stop instead of hammering
    // the endpoint forever.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000)
    })
    expect(listSearchIndexes).toHaveBeenCalledTimes(2)
  })

  it('sends rebuild=false for refresh and rebuild=true for rebuild', async () => {
    listSearchIndexes.mockResolvedValue([row()])
    refreshSearchIndex.mockResolvedValue({ tool: 'zg', root: 'C:/repo', action: 'refresh' })
    const { container } = await renderPanel()

    await act(async () => q(container, 'search-index-refresh')!.click())
    expect(refreshSearchIndex).toHaveBeenLastCalledWith('zg', 'C:/repo', false)

    await act(async () => q(container, 'search-index-rebuild')!.click())
    expect(refreshSearchIndex).toHaveBeenLastCalledWith('zg', 'C:/repo', true)
  })

  it('surfaces a 409 from the backend verbatim instead of swallowing it', async () => {
    listSearchIndexes.mockResolvedValue([row()])
    refreshSearchIndex.mockRejectedValue(
      new Error('bu kök için zaten bir indeks çalışması sürüyor'),
    )
    const { container } = await renderPanel()

    await act(async () => q(container, 'search-index-refresh')!.click())

    expect(q(container, 'search-index-notice')?.textContent).toContain('zaten bir indeks çalışması')
  })

  it('never drops without a confirmation that matches the root exactly', async () => {
    listSearchIndexes.mockResolvedValue([row()])
    const { container } = await renderPanel()

    await act(async () => q(container, 'search-index-drop-open')!.click())
    // Nothing typed yet, and a near-miss, both stay refused client-side.
    expect(disabled(q(container, 'search-index-drop-confirm-btn'))).toBe(true)

    await type(q(container, 'search-index-drop-input') as HTMLInputElement, 'C:/rep')
    expect(disabled(q(container, 'search-index-drop-confirm-btn'))).toBe(true)
    expect(dropSearchIndex).not.toHaveBeenCalled()
  })

  it('drops with the typed root as confirmRoot once it matches', async () => {
    listSearchIndexes.mockResolvedValue([row()])
    dropSearchIndex.mockResolvedValue({ tool: 'zg', root: 'C:/repo', dropped: true })
    const { container } = await renderPanel()

    await act(async () => q(container, 'search-index-drop-open')!.click())
    await type(q(container, 'search-index-drop-input') as HTMLInputElement, 'C:/repo')
    expect(disabled(q(container, 'search-index-drop-confirm-btn'))).toBe(false)
    await act(async () => q(container, 'search-index-drop-confirm-btn')!.click())

    expect(dropSearchIndex).toHaveBeenCalledWith('zg', 'C:/repo', 'C:/repo')
  })

  it('explains an empty list instead of rendering nothing', async () => {
    listSearchIndexes.mockResolvedValue([])

    const { container } = await renderPanel()

    expect(q(container, 'search-index-empty')).toBeTruthy()
  })

  it('reports a failed load through onError', async () => {
    listSearchIndexes.mockRejectedValue(new Error('backend down'))

    const { onError } = await renderPanel()

    expect(onError).toHaveBeenCalledWith('backend down')
  })
})

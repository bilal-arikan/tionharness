// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAsync, type UseAsyncOptions } from './useAsync'
import { useVisiblePoll } from './useVisiblePoll'

const reactEnvironment = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
reactEnvironment.IS_REACT_ACT_ENVIRONMENT = true

let root: ReturnType<typeof createRoot>
let container: HTMLDivElement

function visibility(state: DocumentVisibilityState) {
  Object.defineProperty(document, 'visibilityState', { configurable: true, value: state })
  act(() => document.dispatchEvent(new Event('visibilitychange')))
}

function Poll({ fn, enabled = true }: { fn: () => void; enabled?: boolean }) {
  useVisiblePoll(fn, 1000, [], enabled)
  return null
}

function Async({
  fn,
  version = 0,
  opts = {},
}: {
  fn: () => Promise<string>
  version?: number
  opts?: UseAsyncOptions
}) {
  const { data, loading, error, refresh } = useAsync(fn, [version], opts)
  return <button onClick={refresh}>{JSON.stringify({ data, loading, error })}</button>
}

beforeEach(() => {
  vi.useFakeTimers()
  visibility('visible')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})

afterEach(() => {
  act(() => root.unmount())
  container.remove()
  vi.useRealTimers()
})

describe('shared polling lifecycle', () => {
  it('keeps interval timing when the visible poll callback changes', () => {
    const first = vi.fn()
    const next = vi.fn()
    act(() => root.render(<Poll fn={first} />))
    expect(first).not.toHaveBeenCalled()
    act(() => vi.advanceTimersByTime(500))
    act(() => root.render(<Poll fn={next} />))
    act(() => vi.advanceTimersByTime(500))
    expect(first).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledTimes(1)
  })

  it('starts hidden without a timer, catches up on return and cleans up when disabled', () => {
    const fn = vi.fn()
    visibility('hidden')
    act(() => root.render(<Poll fn={fn} />))
    act(() => vi.advanceTimersByTime(3000))
    expect(fn).not.toHaveBeenCalled()
    visibility('visible')
    expect(fn).toHaveBeenCalledTimes(1)
    act(() => vi.advanceTimersByTime(1000))
    expect(fn).toHaveBeenCalledTimes(2)
    visibility('hidden')
    act(() => vi.advanceTimersByTime(3000))
    expect(fn).toHaveBeenCalledTimes(2)
    act(() => root.render(<Poll fn={fn} enabled={false} />))
    visibility('visible')
    act(() => vi.advanceTimersByTime(3000))
    expect(fn).toHaveBeenCalledTimes(2)
  })

  it('fetches async data immediately while hidden and pauses only subsequent polling', async () => {
    const fn = vi.fn(async () => 'ready')
    visibility('hidden')
    await act(async () => root.render(<Async fn={fn} opts={{ pollMs: 1000 }} />))
    expect(fn).toHaveBeenCalledTimes(1)
    expect(JSON.parse(container.textContent!)).toEqual({
      data: 'ready',
      loading: false,
      error: null,
    })
    await act(async () => vi.advanceTimersByTime(3000))
    expect(fn).toHaveBeenCalledTimes(1)
    await act(async () => visibility('visible'))
    expect(fn).toHaveBeenCalledTimes(2)
  })

  it('allows background polling and stops all polling after unmount', async () => {
    const fn = vi.fn(async () => 'ready')
    visibility('hidden')
    await act(async () =>
      root.render(<Async fn={fn} opts={{ pollMs: 1000, pauseWhenHidden: false }} />),
    )
    await act(async () => vi.advanceTimersByTime(2000))
    expect(fn).toHaveBeenCalledTimes(3)
    act(() => root.unmount())
    visibility('visible')
    await act(async () => vi.advanceTimersByTime(2000))
    expect(fn).toHaveBeenCalledTimes(3)
    // Keep the common cleanup independent from this explicit unmount.
    root = createRoot(container)
  })

  it('ignores superseded async results and exposes errors through manual refresh', async () => {
    let resolveFirst!: (value: string) => void
    const first = () =>
      new Promise<string>((resolve) => {
        resolveFirst = resolve
      })
    await act(async () => root.render(<Async fn={first} />))
    expect(JSON.parse(container.textContent!).loading).toBe(true)
    const next = vi.fn().mockResolvedValueOnce('new').mockRejectedValueOnce(new Error('failed'))
    await act(async () => root.render(<Async fn={next} version={1} />))
    await act(async () => resolveFirst('old'))
    expect(JSON.parse(container.textContent!).data).toBe('new')
    await act(async () => container.querySelector('button')!.click())
    expect(JSON.parse(container.textContent!)).toEqual({
      data: 'new',
      loading: false,
      error: 'failed',
    })
  })

  it('defers async fetches until enabled, including when polling is configured', async () => {
    const fn = vi.fn(async () => 'ready')
    await act(async () => root.render(<Async fn={fn} opts={{ pollMs: 1000, enabled: false }} />))
    await act(async () => vi.advanceTimersByTime(2000))
    expect(fn).not.toHaveBeenCalled()
    await act(async () => root.render(<Async fn={fn} opts={{ pollMs: 1000 }} />))
    expect(fn).toHaveBeenCalledTimes(1)
    await act(async () => vi.advanceTimersByTime(1000))
    expect(fn).toHaveBeenCalledTimes(2)
  })
})

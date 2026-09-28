// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Message } from '@/types'
import { useTranscript } from './useTranscript'

const { listMessagePage } = vi.hoisted(() => ({ listMessagePage: vi.fn() }))
vi.mock('@/api', () => ({ api: { listMessagePage } }))
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const messages: Message[] = Array.from({ length: 600 }, (_, i) => ({
  id: `M${i}`,
  sessionId: 'S1',
  role: 'user',
  text: `message ${i}`,
  createdAt: i,
}))
let root: Root
let state: ReturnType<typeof useTranscript>
const reportError = vi.fn()
function Harness({
  workspace = 'W1',
  sid = 'S1',
  highlight = null,
}: {
  workspace?: string
  sid?: string
  highlight?: string | null
}) {
  state = useTranscript(workspace, sid, highlight, reportError)
  return null
}
async function render(props: Parameters<typeof Harness>[0] = {}) {
  await act(async () => {
    root.render(<Harness {...props} />)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  listMessagePage.mockReset()
  reportError.mockReset()
  listMessagePage.mockImplementation(async (_sid, options = {}) => {
    const limit = options.limit ?? 50
    let from = Math.max(0, messages.length - limit)
    if (options.before) from = Math.max(0, Number(options.before.slice(1)) - limit)
    if (options.after) from = Number(options.after.slice(1)) + 1
    if (options.around) from = Math.max(0, Number(options.around.slice(1)) - Math.floor(limit / 2))
    if (options.start) from = Number(options.start.slice(1))
    const end = options.before
      ? Number(options.before.slice(1))
      : Math.min(messages.length, from + limit)
    return {
      items: messages.slice(from, end),
      offset: from,
      total: messages.length,
      hasMore: from > 0,
      hasNewer: end < messages.length,
    }
  })
  root = createRoot(document.createElement('div'))
})
afterEach(async () => {
  await act(async () => root.unmount())
  vi.useRealTimers()
})

describe('paged transcript ownership', () => {
  it('opens at the live edge and keeps navigation memory bounded', async () => {
    await render()
    expect(state.messages.map((m) => m.id)).toEqual(messages.slice(-50).map((m) => m.id))
    for (let i = 0; i < 5; i++) await act(async () => state.transcriptPaging.loadOlder())
    expect(state.messages.length).toBe(150)
    expect(state.transcriptPaging.hasNewer).toBe(true)
    expect(state.messages[0].id).toBe('M300')
    await act(async () => state.setMessages((previous) => [...previous, messages[599]]))
    expect(state.messages.at(-1)?.id).toBe('M449')
    await act(async () => state.transcriptPaging.loadLatest())
    expect(state.messages.at(-1)?.id).toBe('M599')
    expect(state.transcriptPaging.hasNewer).toBe(false)
  })

  it('coalesces hub completion and workspace notifications into one refresh', async () => {
    await render()
    listMessagePage.mockClear()
    const first = state.refreshMessages('S1')
    const second = state.refreshMessages('S1')
    expect(first).toBe(second)
    await act(async () => {
      await vi.advanceTimersByTimeAsync(75)
      await first
    })
    expect(listMessagePage).toHaveBeenCalledTimes(1)
  })

  it('aborts and rejects a previous workspace response even when session ids match', async () => {
    let finish!: (value: unknown) => void
    listMessagePage.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    await render()
    const oldSignal = listMessagePage.mock.calls[0][2] as AbortSignal
    await render({ workspace: 'W2' })
    expect(oldSignal.aborted).toBe(true)
    await act(async () =>
      finish({
        items: [{ ...messages[0], text: 'wrong workspace' }],
        offset: 0,
        total: 1,
        hasMore: false,
        hasNewer: false,
      }),
    )
    expect(state.messages).toHaveLength(50)
    expect(state.messages.some((m) => m.text === 'wrong workspace')).toBe(false)
    expect(reportError).not.toHaveBeenCalled()
  })

  it('refreshes again when a newer completion arrives during the request', async () => {
    await render()
    let finish!: (value: unknown) => void
    listMessagePage.mockClear()
    listMessagePage.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          finish = resolve
        }),
    )
    const pending = state.refreshMessages('S1')
    await act(async () => {
      await vi.advanceTimersByTimeAsync(75)
    })
    expect(state.refreshMessages('S1')).toBe(pending)
    await act(async () => {
      finish({
        items: messages.slice(-50),
        offset: 550,
        total: 600,
        hasMore: true,
        hasNewer: false,
      })
      await pending
    })
    expect(listMessagePage).toHaveBeenCalledTimes(2)
  })

  it('loads a search target directly and can page forward from it', async () => {
    await render({ highlight: 'M80' })
    expect(state.messages.some((m) => m.id === 'M80')).toBe(true)
    expect(state.messages.length).toBe(50)
    expect(state.transcriptPaging.hasNewer).toBe(true)
    await act(async () => state.transcriptPaging.loadNewer())
    expect(state.messages.at(-1)?.id).toBe('M154')
  })

  it('recovers from a cursor deleted in another window', async () => {
    await render()
    listMessagePage.mockRejectedValueOnce(Object.assign(new Error('deleted'), { status: 404 }))
    await act(async () => state.transcriptPaging.loadOlder())
    expect(state.messages.at(-1)?.id).toBe('M599')
    expect(state.messages.length).toBe(50)
    expect(reportError).not.toHaveBeenCalled()
  })
})

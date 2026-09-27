// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ProcessEntry } from '@/types'
import { ProcessPanel } from './ProcessPanel'

const apiMock = vi.hoisted(() => ({
  listProcesses: vi.fn(),
  stopProcess: vi.fn(),
  subscribeProcesses: vi.fn(),
  subscribeReconnect: vi.fn(),
  getSessionsByIds: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))

// A fixed clock so the duration column is deterministic: the running entry has
// been up for 42s, the finished one ran for 5s.
const NOW = Date.UTC(2026, 8, 23, 12, 0, 0)

const running: ProcessEntry = {
  id: 'p1',
  kind: 'shell',
  command: 'go test ./...',
  pid: 4242,
  status: 'running',
  exitCode: 0,
  startedAt: NOW - 42_000,
  owner: { agentName: 'PM', sessionId: 'ses_1', parentSessionId: 'ses_0' },
  stoppable: true,
}

// Owned by a session that has since been deleted: the panel must still list it,
// without offering a link into a session that is gone.
const orphan: ProcessEntry = {
  id: 'p3',
  kind: 'mcp',
  command: 'codebase-memory-mcp serve',
  status: 'succeeded',
  exitCode: 0,
  startedAt: NOW - 90_000,
  endedAt: NOW - 80_000,
  owner: { sessionId: 'ses_gone' },
  stoppable: false,
}

const finished: ProcessEntry = {
  id: 'p2',
  kind: 'provider',
  command: 'claude --print',
  status: 'failed',
  exitCode: 1,
  error: 'boom',
  startedAt: NOW - 20_000,
  endedAt: NOW - 15_000,
  owner: {},
  outputTail: 'panic: boom',
  stoppable: false,
}

describe('ProcessPanel', () => {
  let container: HTMLDivElement
  let root: Root | null
  // The `process` SSE callback the panel registers, so the test can fire frames.
  let onProcessEvent: () => void

  beforeEach(() => {
    vi.clearAllMocks()
    vi.useFakeTimers()
    vi.setSystemTime(NOW)
    apiMock.listProcesses.mockResolvedValue([running, finished, orphan])
    apiMock.stopProcess.mockResolvedValue({ id: 'p1', stopped: true })
    // ses_gone is absent from the answer: the backend only returns the sessions
    // that still exist.
    apiMock.getSessionsByIds.mockResolvedValue([{ id: 'ses_1' }, { id: 'ses_0' }])
    onProcessEvent = () => {}
    apiMock.subscribeProcesses.mockImplementation((cb: () => void) => {
      onProcessEvent = cb
      return () => {}
    })
    apiMock.subscribeReconnect.mockReturnValue(() => {})
    container = document.createElement('div')
    document.body.appendChild(container)
    root = null
    ;(
      globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true
  })

  afterEach(() => {
    act(() => root?.unmount())
    container.remove()
    vi.useRealTimers()
  })

  const render = async (onOpenSession?: (id: string) => void) => {
    await act(async () => {
      root = createRoot(container)
      root.render(<ProcessPanel onError={() => {}} onOpenSession={onOpenSession} />)
    })
  }

  const rows = () => Array.from(container.querySelectorAll('[data-testid="process-row"]'))
  const cell = (rowIndex: number, testId: string) =>
    rows()[rowIndex].querySelector(`[data-testid="${testId}"]`)?.textContent ?? ''

  it('renders one row per entry with its status and duration', async () => {
    await render()

    expect(rows()).toHaveLength(3)
    expect(cell(0, 'process-status')).toBe('Çalışıyor')
    expect(cell(1, 'process-status')).toBe('Başarısız')
    // Running: measured against "now". Finished: the fixed span it ran for.
    expect(cell(0, 'process-duration')).toMatch(/42/)
    expect(cell(1, 'process-duration')).toMatch(/5/)
    expect(container.textContent).toContain('go test ./...')
    expect(container.textContent).toContain('4242')
    expect(container.textContent).toContain('PM')
  })

  it('links the owner session and its parent, but leaves a deleted session as text', async () => {
    const onOpenSession = vi.fn()
    await render(onOpenSession)

    expect(apiMock.getSessionsByIds).toHaveBeenCalledWith(['ses_0', 'ses_1', 'ses_gone'])

    // Row 0: both the session and the coordinator that spawned it are live.
    const links = rows()[0].querySelectorAll<HTMLButtonElement>(
      '[data-testid="process-session-link"]',
    )
    expect(Array.from(links).map((l) => l.textContent)).toEqual(['ses_1', 'üst: ses_0'])
    act(() => links[0].click())
    expect(onOpenSession).toHaveBeenCalledWith('ses_1')

    // Row 2: the session is gone — still listed, still readable, not a link.
    expect(rows()[2].querySelector('[data-testid="process-session-link"]')).toBeNull()
    expect(rows()[2].textContent).toContain('ses_gone')
  })

  it('does not link any session without a navigation host', async () => {
    await render()

    expect(container.querySelector('[data-testid="process-session-link"]')).toBeNull()
    expect(rows()[0].textContent).toContain('ses_1')
  })

  it('refetches exactly once for a burst of process events, after the debounce', async () => {
    await render()
    expect(apiMock.listProcesses).toHaveBeenCalledTimes(1) // initial load

    // Three frames inside one debounce window (a fan-out of workers starting).
    act(() => {
      onProcessEvent()
      onProcessEvent()
      onProcessEvent()
    })
    expect(apiMock.listProcesses).toHaveBeenCalledTimes(1) // still debounced

    await act(async () => {
      await vi.advanceTimersByTimeAsync(300)
    })
    expect(apiMock.listProcesses).toHaveBeenCalledTimes(2)
  })

  it('stops a stoppable process after confirmation and refetches', async () => {
    await render()

    const stopButton = rows()[0].querySelector<HTMLButtonElement>('[data-testid="process-stop"]')
    expect(stopButton).not.toBeNull()
    // The finished, non-stoppable entry offers no stop action.
    expect(rows()[1].querySelector('[data-testid="process-stop"]')).toBeNull()

    act(() => stopButton!.click())
    const confirmButton = rows()[0].querySelector<HTMLButtonElement>(
      '[data-testid="process-stop-confirm"]',
    )
    expect(confirmButton).not.toBeNull()
    expect(apiMock.stopProcess).not.toHaveBeenCalled() // first click only arms it

    await act(async () => {
      confirmButton!.click()
    })
    expect(apiMock.stopProcess).toHaveBeenCalledWith('p1')
    expect(apiMock.listProcesses).toHaveBeenCalledTimes(2) // initial + post-stop
  })

  it('surfaces a failed load instead of showing an empty list', async () => {
    apiMock.listProcesses.mockRejectedValue(new Error('PROCESS_LIST_FAILED'))
    await render()

    expect(container.querySelector('[role="alert"]')?.textContent).toContain('PROCESS_LIST_FAILED')
    expect(rows()).toHaveLength(0)
  })
})

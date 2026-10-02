// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, expect, it, vi } from 'vitest'
import { ConnectionNotice } from './ConnectionNotice'
import { i18next } from '@/i18n'

const connection = vi.hoisted(() => ({
  status: 'connecting',
  listeners: new Set<() => void>(),
  retry: vi.fn(),
}))
vi.mock('@/api/liveConnection', () => ({
  getLiveConnectionStatus: () => connection.status,
  subscribeLiveConnectionStatus: (cb: () => void) => {
    connection.listeners.add(cb)
    return () => {
      connection.listeners.delete(cb)
    }
  },
  reconnectLiveConnection: connection.retry,
}))

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true
const roots: ReturnType<typeof createRoot>[] = []
afterEach(() => {
  roots.splice(0).forEach((root) => act(() => root.unmount()))
  document.body.replaceChildren()
  vi.useRealTimers()
})

it('shows a delayed reconnect notice, retries manually, and clears the notice on recovery', async () => {
  vi.useFakeTimers()
  await i18next.changeLanguage('en')
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(<ConnectionNotice />))
  expect(container.textContent).toBe('')
  await act(async () => {
    await vi.advanceTimersByTimeAsync(2000)
  })
  expect(container.querySelector('[role="status"]')?.textContent).toContain(
    'Reconnecting automatically',
  )
  act(() => container.querySelector('button')!.click())
  expect(connection.retry).toHaveBeenCalledTimes(1)
  act(() => {
    connection.status = 'connected'
    connection.listeners.forEach((cb) => cb())
  })
  expect(container.textContent).toBe('')
})

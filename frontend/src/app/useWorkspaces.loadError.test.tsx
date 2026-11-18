// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useWorkspaces } from './useWorkspaces'
import { resolveBootScreen } from './bootScreen'
import { retryDelayMs } from './useRetryingLoad'

const mocks = vi.hoisted(() => ({
  listWorkspaces: vi.fn(),
  reloadApp: vi.fn(),
  setError: vi.fn(),
}))
vi.mock('@/api', () => ({
  api: { listWorkspaces: mocks.listWorkspaces },
  setActiveWorkspace: vi.fn(),
  getActiveWorkspace: () => null,
  clearActiveWorkspace: vi.fn(),
}))
vi.mock('./reloadApp', () => ({ reloadApp: mocks.reloadApp }))
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

type Hook = ReturnType<typeof useWorkspaces>
const roots: ReturnType<typeof createRoot>[] = []

function renderHook() {
  const latest: { current: Hook | null } = { current: null }
  function Probe() {
    latest.current = useWorkspaces(mocks.setError)
    return null
  }
  const root = createRoot(document.createElement('div'))
  roots.push(root)
  act(() => root.render(<Probe />))
  return () => latest.current!
}

const bootScreen = (hook: Hook) =>
  resolveBootScreen({
    hadSetupAtBoot: true,
    wsLoading: hook.loading,
    minSplashElapsed: true,
    loadError: hook.loadError,
    workspaceCount: hook.workspaces.length,
  })

beforeEach(() => {
  vi.useFakeTimers()
  Object.values(mocks).forEach((mock) => mock.mockReset())
})
afterEach(() => {
  roots.splice(0).forEach((root) => act(() => root.unmount()))
  vi.useRealTimers()
  localStorage.clear()
})

it('reports a failed workspace list as a load error instead of an empty list', async () => {
  mocks.listWorkspaces.mockRejectedValue(new Error('Backend unavailable (502)'))
  const hook = renderHook()
  expect(hook().loading).toBe(true)

  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(hook().loading).toBe(false)
  expect(hook().loadError).toBe('Backend unavailable (502)')
  expect(hook().workspaces).toEqual([])
  expect(bootScreen(hook())).toBe('unreachable')
  // The dedicated screen carries the message; no duplicate error toast.
  expect(mocks.setError).not.toHaveBeenCalled()
})

it('retries with backoff and restarts the client once the backend answers', async () => {
  mocks.listWorkspaces
    .mockRejectedValueOnce(new Error('down'))
    .mockRejectedValueOnce(new Error('still down'))
    .mockResolvedValue([{ id: 'WS1', name: 'One' }])
  const hook = renderHook()
  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(mocks.listWorkspaces).toHaveBeenCalledTimes(1)

  await act(() => vi.advanceTimersByTimeAsync(retryDelayMs(1)))
  expect(mocks.listWorkspaces).toHaveBeenCalledTimes(2)
  expect(hook().loadError).toBe('still down')
  expect(mocks.reloadApp).not.toHaveBeenCalled()

  // A manual retry does not wait for the next backoff step.
  act(() => hook().retryLoad())
  await act(() => vi.advanceTimersByTimeAsync(0))
  expect(mocks.listWorkspaces).toHaveBeenCalledTimes(3)
  expect(mocks.reloadApp).toHaveBeenCalledTimes(1)
})

it('treats an answered empty list as a fresh install', async () => {
  mocks.listWorkspaces.mockResolvedValue([])
  const hook = renderHook()
  await act(() => vi.advanceTimersByTimeAsync(0))

  expect(hook().loadError).toBeNull()
  expect(bootScreen(hook())).toBe('onboarding')
  expect(mocks.reloadApp).not.toHaveBeenCalled()
})

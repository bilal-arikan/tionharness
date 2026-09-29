// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { DashboardPanel } from './DashboardPanel'
import { CommitHeatmap } from './CommitHeatmap'

const api = vi.hoisted(() => ({
  getDashboard: vi.fn(),
  getCommitActivity: vi.fn(),
}))
vi.mock('@/api', () => ({ api }))

let root: Root
let container: HTMLDivElement
let unmounted = false

beforeEach(() => {
  vi.clearAllMocks()
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  api.getDashboard.mockImplementation(() => new Promise(() => {}))
  api.getCommitActivity.mockImplementation(() => new Promise(() => {}))
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  unmounted = false
})

afterEach(() => {
  if (!unmounted) act(() => root.unmount())
  container.remove()
})

it('cancels the previous dashboard request when the range changes and on unmount', async () => {
  await act(async () => root.render(<DashboardPanel />))
  const firstSignal = api.getDashboard.mock.calls[0][1] as AbortSignal
  expect(firstSignal.aborted).toBe(false)

  await act(async () => {
    const range = [...container.querySelectorAll('button')].find((button) =>
      button.textContent?.startsWith('30'),
    )!
    range.click()
  })

  const secondSignal = api.getDashboard.mock.calls[1][1] as AbortSignal
  expect(firstSignal.aborted).toBe(true)
  expect(secondSignal.aborted).toBe(false)

  act(() => root.unmount())
  unmounted = true
  expect(secondSignal.aborted).toBe(true)
})

it('cancels commit activity collection when its panel closes', async () => {
  await act(async () => root.render(<CommitHeatmap />))
  const signal = api.getCommitActivity.mock.calls[0][1] as AbortSignal
  expect(signal.aborted).toBe(false)

  act(() => root.unmount())
  unmounted = true
  expect(signal.aborted).toBe(true)
})

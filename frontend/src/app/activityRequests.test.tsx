// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useActivity } from './useActivity'
import { useWorkspaceActivity } from './useWorkspaceActivity'
import { bumpSignal } from '@/shared/lib/refreshSignals'
import {
  SIGNAL_ACTIVITY,
  SIGNAL_EXECUTIONS,
  SIGNAL_WORKSPACE_ACTIVITY,
} from './eventToRefreshSignals'

const calls = vi.hoisted(() => ({ activity: vi.fn(), workspaces: vi.fn() }))
vi.mock('@/api', () => ({
  api: { getActivity: calls.activity, listWorkspacesActivity: calls.workspaces },
}))

let root: Root
function Harness({ workspace }: { workspace: string }) {
  useActivity(workspace, false)
  useWorkspaceActivity(workspace)
  return null
}

beforeEach(() => {
  vi.clearAllMocks()
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  calls.activity.mockResolvedValue({})
  calls.workspaces.mockResolvedValue([])
  root = createRoot(document.createElement('div'))
})
afterEach(() => act(() => root.unmount()))

it('loads once at mount and workspace switch even when signals already advanced', async () => {
  bumpSignal(SIGNAL_ACTIVITY)
  bumpSignal(SIGNAL_EXECUTIONS)
  bumpSignal(SIGNAL_WORKSPACE_ACTIVITY)
  await act(async () => root.render(<Harness workspace="WS1" />))
  expect(calls.activity).toHaveBeenCalledTimes(1)
  expect(calls.workspaces).toHaveBeenCalledTimes(1)
  await act(async () => root.render(<Harness workspace="WS2" />))
  expect(calls.activity).toHaveBeenCalledTimes(2)
  expect(calls.workspaces).toHaveBeenCalledTimes(2)
})

it('still refreshes after real activity and workspace signals', async () => {
  await act(async () => root.render(<Harness workspace="WS1" />))
  await act(async () => bumpSignal(SIGNAL_ACTIVITY))
  expect(calls.activity).toHaveBeenCalledTimes(2)
  expect(calls.workspaces).toHaveBeenCalledTimes(1)
  await act(async () => {
    bumpSignal(SIGNAL_EXECUTIONS)
    bumpSignal(SIGNAL_WORKSPACE_ACTIVITY)
  })
  expect(calls.workspaces).toHaveBeenCalledTimes(2)
})

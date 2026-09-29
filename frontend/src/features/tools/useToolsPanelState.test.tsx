// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import type { WorkspaceTools } from '@/types'

const apiMock = vi.hoisted(() => ({
  listMCPServers: vi.fn(),
  mcpPoolStats: vi.fn(),
  workspaceTools: vi.fn(),
  setWorkspaceTools: vi.fn(),
}))

vi.mock('@/api', () => ({ api: apiMock }))
vi.mock('@/api/workspaceStream', () => ({ subscribeWorkspaceStream: () => () => {} }))
vi.mock('@/shared/hooks/useVisiblePoll', () => ({ useVisiblePoll: () => {} }))

import { useToolsPanelState } from './useToolsPanelState'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

const onError = vi.fn()
let state: ReturnType<typeof useToolsPanelState>

function Harness() {
  state = useToolsPanelState(onError)
  return null
}

let root: Root
let container: HTMLDivElement

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  localStorage.clear()
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  apiMock.listMCPServers.mockResolvedValue([])
  apiMock.mcpPoolStats.mockResolvedValue({ servers: [] })
  apiMock.setWorkspaceTools.mockResolvedValue({ disabledTools: [] })
})

afterEach(async () => {
  await act(async () => root.unmount())
  container.remove()
})

it('shows cached tools while a cold MCP catalog is still loading', async () => {
  const full = deferred<WorkspaceTools>()
  const builtIn = { name: 'Read', enabled: true, visibility: 'full' }
  const mcp = { name: 'example__search', enabled: true, visibility: 'name-only' }
  apiMock.workspaceTools.mockImplementation((cached: boolean) =>
    cached
      ? Promise.resolve({ tools: [builtIn], disabledTools: [], toolVisibility: {} })
      : full.promise,
  )

  await act(async () => root.render(<Harness />))

  expect(apiMock.workspaceTools).toHaveBeenCalledWith(true)
  expect(apiMock.workspaceTools).toHaveBeenCalledWith()
  expect(state.tools.map((tool) => tool.name)).toEqual(['Read'])

  await act(async () => {
    full.resolve({
      tools: [builtIn, mcp] as WorkspaceTools['tools'],
      disabledTools: [],
      toolVisibility: {},
    })
  })

  expect(state.tools.map((tool) => tool.name)).toEqual(['Read', 'example__search'])
  expect(onError).not.toHaveBeenCalled()
})

it('keeps a tool toggle when an older catalog request finishes later', async () => {
  const full = deferred<WorkspaceTools>()
  const builtIn = { name: 'Read', enabled: true, visibility: 'full' }
  const updated = { ...builtIn, enabled: false }
  let fullRequests = 0
  apiMock.workspaceTools.mockImplementation((cached: boolean) => {
    if (cached) {
      return Promise.resolve({
        tools: [fullRequests ? updated : builtIn],
        disabledTools: fullRequests ? ['Read'] : [],
        toolVisibility: {},
      })
    }
    fullRequests++
    return fullRequests === 1
      ? full.promise
      : Promise.resolve({ tools: [updated], disabledTools: ['Read'], toolVisibility: {} })
  })

  await act(async () => root.render(<Harness />))
  await act(async () => state.toggleTool(state.tools[0]))
  expect(state.tools[0].enabled).toBe(false)

  await act(async () => {
    full.resolve({
      tools: [builtIn] as WorkspaceTools['tools'],
      disabledTools: [],
      toolVisibility: {},
    })
  })

  expect(state.tools[0].enabled).toBe(false)
  expect(fullRequests).toBe(2)
  expect(apiMock.setWorkspaceTools).toHaveBeenCalledWith(['Read'])
})

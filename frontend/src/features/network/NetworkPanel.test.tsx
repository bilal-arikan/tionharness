// @vitest-environment jsdom

import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceGraph } from '@/types'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => {
    resolve = done
  })
  return { promise, resolve }
}

const mocks = vi.hoisted(() => ({
  workspaceGraph: vi.fn(),
  getWorkspaceSettings: vi.fn(),
  graphProps: [] as Array<{
    canonicalNodeIds: readonly string[]
    canonicalReady: boolean
  }>,
}))

vi.mock('@/api', () => ({
  api: {
    workspaceGraph: mocks.workspaceGraph,
    getWorkspaceSettings: mocks.getWorkspaceSettings,
  },
}))
vi.mock('@/shared/hooks/useRefreshTrigger', () => ({ useRefreshTrigger: () => 0 }))
vi.mock('@/shared/hooks/useMediaQuery', () => ({ useIsMobile: () => false }))
vi.mock('./VisNetworkGraph', () => ({
  VisNetworkGraph: (props: { canonicalNodeIds: readonly string[]; canonicalReady: boolean }) => {
    mocks.graphProps.push(props)
    return <div data-testid="network" />
  },
}))
vi.mock('./NetworkFilters', () => ({
  NetworkFilters: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}))

import { NetworkPanel } from './NetworkPanel'

const roots = new Set<Root>()

function workspaceGraph(id: string): WorkspaceGraph {
  return {
    nodes: [{ id: `task:${id}`, type: 'task', label: id, status: 'todo' }],
    edges: [],
    stats: { agents: 0, agentsTotal: 0, tasks: 1, flows: 0, skills: 0, mcp: 0 },
  }
}

beforeEach(() => {
  mocks.workspaceGraph.mockReset()
  mocks.getWorkspaceSettings.mockReset()
  mocks.graphProps.length = 0
  const root = document.documentElement
  root.style.setProperty('--color-bg', '#000')
  root.style.setProperty('--color-surface', '#111')
  root.style.setProperty('--color-surface-2', '#222')
  root.style.setProperty('--color-border', '#333')
  root.style.setProperty('--color-text', '#fff')
  root.style.setProperty('--color-text-dim', '#aaa')
  root.style.setProperty('--color-accent', '#0af')
  root.style.setProperty('--color-on-accent', '#000')
})

afterEach(async () => {
  for (const root of [...roots]) {
    await act(async () => root.unmount())
    roots.delete(root)
  }
})

describe('NetworkPanel loading', () => {
  it('keeps the latest request authoritative when responses resolve in reverse order', async () => {
    const oldGraph = deferred<WorkspaceGraph>()
    const newGraph = deferred<WorkspaceGraph>()
    const oldSettings = deferred<{ boardColumns?: [] }>()
    const newSettings = deferred<{ boardColumns?: [] }>()
    mocks.workspaceGraph.mockReturnValueOnce(oldGraph.promise).mockReturnValueOnce(newGraph.promise)
    mocks.getWorkspaceSettings
      .mockReturnValueOnce(oldSettings.promise)
      .mockReturnValueOnce(newSettings.promise)

    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    roots.add(root)
    await act(async () => root.render(<NetworkPanel workspaceId="workspace" onError={vi.fn()} />))

    expect(mocks.workspaceGraph).toHaveBeenCalledTimes(2)
    expect(mocks.graphProps.at(-1)).toMatchObject({ canonicalReady: false })

    await act(async () => {
      newGraph.resolve(workspaceGraph('new'))
      newSettings.resolve({ boardColumns: [] })
      await newGraph.promise
      await Promise.resolve()
    })
    expect(mocks.graphProps.at(-1)).toMatchObject({ canonicalReady: true })
    expect(mocks.graphProps.at(-1)?.canonicalNodeIds).toContain('task:new')

    await act(async () => {
      oldGraph.resolve(workspaceGraph('old'))
      oldSettings.resolve({ boardColumns: [] })
      await oldGraph.promise
      await Promise.resolve()
    })
    expect(mocks.graphProps.at(-1)?.canonicalNodeIds).toContain('task:new')
    expect(mocks.graphProps.at(-1)?.canonicalNodeIds).not.toContain('task:old')
    expect(mocks.graphProps.at(-1)).toMatchObject({ canonicalReady: true })
  })
})

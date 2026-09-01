// @vitest-environment jsdom

import { act, type ReactNode } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { WorkspaceGraph } from '@/types'

const reactActEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactActEnvironment.IS_REACT_ACT_ENVIRONMENT = true

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((done, fail) => {
    resolve = done
    reject = fail
  })
  return { promise, resolve, reject }
}

const mocks = vi.hoisted(() => ({
  workspaceGraph: vi.fn(),
  getWorkspaceSettings: vi.fn(),
  graphProps: [] as Array<{
    workspaceId: string
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
  VisNetworkGraph: (props: {
    workspaceId: string
    canonicalNodeIds: readonly string[]
    canonicalReady: boolean
  }) => {
    mocks.graphProps.push(props)
    return <div data-testid="network" data-workspace={props.workspaceId} />
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
  it('suppresses an old rejection during workspace commit but reports current failures', async () => {
    const oldWorkspaceRefresh = deferred<WorkspaceGraph>()
    const onError = vi.fn()
    mocks.workspaceGraph
      .mockResolvedValueOnce(workspaceGraph('a-discarded'))
      .mockResolvedValueOnce(workspaceGraph('a-ready'))
      .mockReturnValueOnce(oldWorkspaceRefresh.promise)
      .mockResolvedValueOnce(workspaceGraph('b-discarded'))
      .mockResolvedValueOnce(workspaceGraph('b-ready'))
      .mockRejectedValueOnce(new Error('current workspace failed'))
    mocks.getWorkspaceSettings.mockResolvedValue({ boardColumns: [] })

    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    roots.add(root)
    await act(async () => root.render(<NetworkPanel workspaceId="workspace-a" onError={onError} />))
    await act(async () => Promise.resolve())

    await act(async () => host.querySelector<HTMLButtonElement>('button[title="Yenile"]')?.click())
    const workspaceCommit = new Promise<void>((resolve) => {
      const observer = new MutationObserver(() => {
        if (host.querySelector('[data-workspace="workspace-b"]')) {
          observer.disconnect()
          oldWorkspaceRefresh.reject(new Error('old workspace failed'))
          resolve()
        }
      })
      observer.observe(host, { attributes: true, subtree: true })
    })

    reactActEnvironment.IS_REACT_ACT_ENVIRONMENT = false
    root.render(<NetworkPanel workspaceId="workspace-b" onError={onError} />)
    reactActEnvironment.IS_REACT_ACT_ENVIRONMENT = true
    await workspaceCommit
    await Promise.resolve()
    await act(async () => Promise.resolve())

    expect(onError).not.toHaveBeenCalled()
    expect(mocks.graphProps.at(-1)).toMatchObject({
      workspaceId: 'workspace-b',
      canonicalReady: true,
      canonicalNodeIds: expect.arrayContaining(['task:b-ready']),
    })

    await act(async () => host.querySelector<HTMLButtonElement>('button[title="Yenile"]')?.click())
    await act(async () => Promise.resolve())

    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith('current workspace failed')
  })

  it('keeps readiness and canonical ids sticky while a refresh is unresolved', async () => {
    const refreshGraph = deferred<WorkspaceGraph>()
    const refreshSettings = deferred<{ boardColumns?: [] }>()
    mocks.workspaceGraph
      .mockResolvedValueOnce(workspaceGraph('initial-discarded'))
      .mockResolvedValueOnce(workspaceGraph('ready'))
      .mockReturnValueOnce(refreshGraph.promise)
    mocks.getWorkspaceSettings
      .mockResolvedValueOnce({ boardColumns: [] })
      .mockResolvedValueOnce({ boardColumns: [] })
      .mockReturnValueOnce(refreshSettings.promise)

    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    roots.add(root)
    await act(async () => root.render(<NetworkPanel workspaceId="workspace" onError={vi.fn()} />))
    await act(async () => Promise.resolve())

    expect(mocks.graphProps.at(-1)).toMatchObject({
      canonicalReady: true,
      canonicalNodeIds: expect.arrayContaining(['task:ready']),
    })

    await act(async () => host.querySelector<HTMLButtonElement>('button[title="Yenile"]')?.click())

    expect(mocks.graphProps.at(-1)).toMatchObject({
      canonicalReady: true,
      canonicalNodeIds: expect.arrayContaining(['task:ready']),
    })
  })

  it('does not expose an old workspace while the new workspace is loading', async () => {
    const lateWorkspaceAGraph = deferred<WorkspaceGraph>()
    const lateWorkspaceASettings = deferred<{ boardColumns?: [] }>()
    const workspaceBGraph = deferred<WorkspaceGraph>()
    const workspaceBSettings = deferred<{ boardColumns?: [] }>()
    mocks.workspaceGraph
      .mockResolvedValueOnce(workspaceGraph('a-discarded'))
      .mockResolvedValueOnce(workspaceGraph('a'))
      .mockReturnValueOnce(lateWorkspaceAGraph.promise)
      .mockReturnValueOnce(workspaceBGraph.promise)
      .mockReturnValueOnce(workspaceBGraph.promise)
    mocks.getWorkspaceSettings
      .mockResolvedValueOnce({ boardColumns: [] })
      .mockResolvedValueOnce({ boardColumns: [] })
      .mockReturnValueOnce(lateWorkspaceASettings.promise)
      .mockReturnValueOnce(workspaceBSettings.promise)
      .mockReturnValueOnce(workspaceBSettings.promise)

    const host = document.createElement('div')
    document.body.appendChild(host)
    const root = createRoot(host)
    roots.add(root)
    await act(async () => root.render(<NetworkPanel workspaceId="workspace-a" onError={vi.fn()} />))
    await act(async () => Promise.resolve())
    expect(mocks.graphProps.at(-1)).toMatchObject({ canonicalReady: true })

    await act(async () => host.querySelector<HTMLButtonElement>('button[title="Yenile"]')?.click())

    await act(async () => root.render(<NetworkPanel workspaceId="workspace-b" onError={vi.fn()} />))

    expect(mocks.graphProps.at(-1)).toMatchObject({
      workspaceId: 'workspace-b',
      canonicalReady: false,
      canonicalNodeIds: [],
    })

    await act(async () => {
      workspaceBGraph.resolve(workspaceGraph('b'))
      workspaceBSettings.resolve({ boardColumns: [] })
      await workspaceBGraph.promise
      await Promise.resolve()
    })
    expect(mocks.graphProps.at(-1)).toMatchObject({
      workspaceId: 'workspace-b',
      canonicalReady: true,
      canonicalNodeIds: expect.arrayContaining(['task:b']),
    })

    await act(async () => {
      lateWorkspaceAGraph.resolve(workspaceGraph('late-a'))
      lateWorkspaceASettings.resolve({ boardColumns: [] })
      await lateWorkspaceAGraph.promise
      await Promise.resolve()
    })
    expect(mocks.graphProps.at(-1)).toMatchObject({
      workspaceId: 'workspace-b',
      canonicalReady: true,
      canonicalNodeIds: expect.arrayContaining(['task:b']),
    })
    expect(mocks.graphProps.at(-1)?.canonicalNodeIds).not.toContain('task:late-a')
  })

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

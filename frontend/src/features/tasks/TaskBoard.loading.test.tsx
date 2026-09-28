// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { TaskBoard } from './TaskBoard'

const api = vi.hoisted(() => ({
  listTasks: vi.fn(),
  listFlows: vi.fn(),
  listArtifacts: vi.fn(),
  getWorkspaceSettings: vi.fn(),
}))
vi.mock('@/api', () => ({ api, getActiveWorkspace: () => 'WS5' }))
vi.mock('@/features/view/ViewButton', () => ({ ViewButton: () => null }))
vi.mock('./TaskFormModal', () => ({ TaskFormModal: () => null }))
vi.mock('./BoardColumnEditor', () => ({ BoardColumnEditor: () => null }))
let root: Root
let container: HTMLDivElement
beforeEach(() => {
  vi.clearAllMocks()
  localStorage.clear()
  ;(
    globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
  ).IS_REACT_ACT_ENVIRONMENT = true
  api.listTasks.mockResolvedValue([])
  api.listFlows.mockResolvedValue([])
  api.listArtifacts.mockResolvedValue({ items: [] })
  api.getWorkspaceSettings.mockResolvedValue({})
  container = document.createElement('div')
  document.body.appendChild(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  container.remove()
})

it('loads the active board once and fetches archives only after the toggle', async () => {
  await act(async () => root.render(<TaskBoard agents={[]} onError={vi.fn()} />))
  expect(api.listTasks).toHaveBeenCalledExactlyOnceWith(false)
  expect(api.getWorkspaceSettings).toHaveBeenCalledTimes(1)
  expect(api.listArtifacts).toHaveBeenCalledTimes(1)
  await act(async () =>
    container
      .querySelector<HTMLButtonElement>('[data-testid="task-board-archived-toggle"]')!
      .click(),
  )
  expect(api.listTasks).toHaveBeenCalledTimes(2)
  expect(api.listTasks).toHaveBeenLastCalledWith('only')
})

it('ignores an active response that arrives after switching to archives', async () => {
  let resolveActive!: (rows: unknown[]) => void
  api.listTasks.mockImplementation((side: unknown) =>
    side === false
      ? new Promise((resolve) => {
          resolveActive = resolve
        })
      : Promise.resolve([]),
  )
  await act(async () => root.render(<TaskBoard agents={[]} onError={vi.fn()} />))
  await act(async () =>
    container
      .querySelector<HTMLButtonElement>('[data-testid="task-board-archived-toggle"]')!
      .click(),
  )
  await act(async () =>
    resolveActive([
      { id: 'TSK1', title: 'Stale active task', boardState: 'pbi', createdAt: 1, updatedAt: 1 },
    ]),
  )
  expect(container.textContent).not.toContain('Stale active task')
})

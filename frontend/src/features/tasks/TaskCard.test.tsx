// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Task } from '@/types'
import { TaskCard, type TaskCardMeta } from './TaskCard'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

const task: Task = {
  id: 'TSK548',
  title: 'Kart kimliğini göster',
  description: '',
  prompt: '',
  ownerAgentId: '',
  flowId: '',
  boardState: 'todo',
  dependencies: '',
  lastRunId: '',
  lastRunStatus: '',
  lastRunAt: 0,
  createdAt: 0,
  updatedAt: 0,
}

const meta: TaskCardMeta = {
  owner: undefined,
  flow: undefined,
  depIds: [],
  unmetDeps: [],
  unmetColColor: null,
  image: null,
}

function renderCard(overrides: Partial<Task> = {}, onUnarchive?: (task: Task) => void) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(
      <TaskCard
        task={{ ...task, ...overrides }}
        meta={meta}
        selected={false}
        fileDropActive={false}
        recentlyChanged={false}
        columnIndex={0}
        columnCount={3}
        columnLabel="Yapılacak"
        onDragStart={vi.fn()}
        onDragEnd={vi.fn()}
        onOpenOrSelect={vi.fn()}
        onFileDragEnter={vi.fn()}
        onFileDragLeave={vi.fn()}
        onFileDrop={vi.fn()}
        onMoveColumn={vi.fn()}
        onUnarchive={onUnarchive}
      />,
    ),
  )
  return container
}

describe('TaskCard task id', () => {
  it('positions the real task id at the top right and includes it in the accessible name', () => {
    const container = renderCard()
    const card = container.querySelector<HTMLElement>('[data-testid="task-card"]')
    const id = Array.from(container.querySelectorAll('span')).find(
      (element) => element.textContent === 'TSK548',
    )

    expect(id).toBeDefined()
    expect(card?.className).toContain('relative')
    expect(card?.className).toContain('p-2')
    expect(card?.className).not.toMatch(/(?:^|\s)pt-/)
    expect(id?.className).toContain('absolute')
    expect(id?.className).toContain('right-2')
    expect(id?.className).toContain('top-2')
    expect(id?.className).toContain('text-[var(--color-text-dim)]')
    expect(id?.className).toContain('opacity-70')
    expect(id?.nextElementSibling?.className).toContain('pr-12')
    expect(card?.getAttribute('aria-label')).toContain('görev kimliği TSK548')
  })

  it('keeps the restore action in normal flow, separate from the positioned id', () => {
    const container = renderCard({}, vi.fn())
    const restore = container.querySelector<HTMLButtonElement>('button[title^="Arşivden çıkar"]')
    const id = Array.from(container.querySelectorAll('span')).find(
      (element) => element.textContent === 'TSK548',
    )

    expect(restore).not.toBeNull()
    expect(restore?.parentElement).not.toBe(id?.parentElement)
    expect(restore?.parentElement?.textContent).not.toContain('TSK548')
    expect(restore?.className).not.toContain('absolute')
  })

  it('does not present an optimistic temporary id as a real task id', () => {
    const container = renderCard({ id: 'temp-123' })
    const card = container.querySelector<HTMLElement>('[data-testid="task-card"]')

    expect(container.textContent).not.toContain('temp-123')
    expect(card?.getAttribute('aria-label')).not.toContain('görev kimliği')
  })
})

// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Session } from '@/types'
import { ALL_SESSION_CHIPS } from './sessionKindMeta'
import { SessionsSidebar } from './SessionsSidebar'
import { i18next } from '@/i18n'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const FILTERS_OPEN_KEY = 'tionharness.filtersOpen.sessions'
const roots: ReturnType<typeof createRoot>[] = []

beforeEach(async () => {
  localStorage.clear()
  await i18next.changeLanguage('tr')
})

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
  localStorage.clear()
})

function session(overrides: Partial<Session> = {}): Session {
  return {
    id: 'SES42',
    kind: 'chat',
    state: 'active',
    agentId: '',
    title: 'Weekly plan',
    createdAt: 1,
    updatedAt: 1,
    messageCount: 2,
    ...overrides,
  } as Session
}

function render(sessions: Session[], chipCountsFromServer: Record<string, number> = {}) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() =>
    root.render(
      <SessionsSidebar
        sessions={sessions}
        agents={[]}
        activeSessionId={null}
        newDisabled={false}
        chipsOff={[]}
        chipSet={new Set(ALL_SESSION_CHIPS)}
        onClickChip={vi.fn()}
        chipCountsFromServer={chipCountsFromServer}
        showArchived={false}
        onToggleArchived={vi.fn()}
        onSelectSession={vi.fn()}
        onNewSession={vi.fn()}
        onRefresh={vi.fn()}
        onGenerateTitle={vi.fn()}
        onDeleteSession={vi.fn()}
        onSetArchived={vi.fn()}
        onSetPinned={vi.fn()}
      />,
    ),
  )
  return container
}

describe('SessionsSidebar filter chips', () => {
  it('keeps the chip strip folded until the user opens it', () => {
    const container = render([session()])
    expect(container.querySelector('[data-testid="session-kind-filters"]')).toBeNull()

    const toggle = container.querySelector<HTMLButtonElement>(
      '[data-testid="session-filters-toggle"]',
    )!
    act(() => toggle.click())
    expect(container.querySelector('[data-testid="session-kind-filters"]')).not.toBeNull()
    expect(localStorage.getItem(FILTERS_OPEN_KEY)).toBe('1')
  })

  it('honours a remembered open strip', () => {
    localStorage.setItem(FILTERS_OPEN_KEY, '1')
    const container = render([session()])
    expect(container.querySelector('[data-testid="session-kind-filters"]')).not.toBeNull()
  })

  it('accents only ticked chips that cover sessions', () => {
    localStorage.setItem(FILTERS_OPEN_KEY, '1')
    const container = render([session()], { chat: 3 })
    const chat = container.querySelector<HTMLElement>('[data-chip="chat"]')!
    const task = container.querySelector<HTMLElement>('[data-chip="task"]')!

    expect(chat.getAttribute('aria-pressed')).toBe('true')
    expect(chat.dataset.empty).toBeUndefined()
    expect(chat.className).toContain('--color-accent')

    expect(task.getAttribute('aria-pressed')).toBe('true')
    expect(task.dataset.empty).toBe('true')
    expect(task.className).not.toContain('--color-accent')
  })
})

describe('SessionsSidebar row', () => {
  it('gives the title its own line and moves the spawn marker into the meta line', () => {
    const container = render([session({ kind: 'spawned', title: 'Task cluster A' })])
    const row = [...container.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
      button.textContent?.includes('Task cluster A'),
    )!
    const meta = row.querySelector<HTMLElement>('[data-testid="session-row-meta"]')!
    const origin = row.querySelector<HTMLElement>('[data-testid="session-row-origin"]')!

    expect(meta.contains(origin)).toBe(true)
    expect(origin.textContent).toContain('spawn')
    expect(meta.textContent).not.toContain('Task cluster A')
  })

  it('reveals the session id on hover or focus and always carries it in the tooltip', () => {
    const container = render([session()])
    const row = [...container.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
      button.textContent?.includes('Weekly plan'),
    )!
    const id = row.querySelector<HTMLElement>('[data-testid="session-row-id"]')!

    expect(row.title).toBe('Weekly plan · SES42')
    expect(id.textContent).toBe('SES42')
    expect(id.className).toContain('hidden')
    expect(id.className).toContain('group-hover:inline')
    expect(id.className).toContain('group-focus-within:inline')
  })

  it('titles the column and its buckets in sentence case', () => {
    const container = render([session()])
    expect(container.querySelector('.uppercase')).toBeNull()
    expect(container.querySelector('[class*="tracking-"]')).toBeNull()
  })
})

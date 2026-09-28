// @vitest-environment jsdom

import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Session } from '@/types'
import { ALL_SESSION_CHIPS } from './sessionKindMeta'
import { SessionsSidebar } from './SessionsSidebar'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: ReturnType<typeof createRoot>[] = []

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

function session(id: string, state: 'active' | 'archived'): Session {
  return {
    id,
    kind: 'chat',
    state,
    agentId: '',
    title: id,
    createdAt: 1,
    updatedAt: 1,
    messageCount: 0,
  } as Session
}

describe('SessionsSidebar archive view', () => {
  it('switches sides from the header and restores selected archived sessions', () => {
    const container = document.createElement('div')
    document.body.appendChild(container)
    const root = createRoot(container)
    roots.push(root)
    const onToggleArchived = vi.fn()
    const onSetArchived = vi.fn()
    const props = {
      sessions: [session('ACTIVE', 'active'), session('ARCHIVED', 'archived')],
      agents: [],
      activeSessionId: null,
      newDisabled: false,
      chipsOff: [],
      chipSet: new Set(ALL_SESSION_CHIPS),
      onClickChip: vi.fn(),
      onToggleArchived,
      onSelectSession: vi.fn(),
      onNewSession: vi.fn(),
      onRefresh: vi.fn(),
      onGenerateTitle: vi.fn(),
      onDeleteSession: vi.fn(),
      onSetArchived,
      onSetPinned: vi.fn(),
    }

    act(() => root.render(<SessionsSidebar {...props} showArchived={false} />))
    expect(container.querySelector('[data-chip="archived"]')).toBeNull()
    expect(container.textContent).toContain('ACTIVE')
    expect(container.textContent).not.toContain('ARCHIVED')

    const toggle = container.querySelector<HTMLButtonElement>(
      '[data-testid="sessions-archived-toggle"]',
    )!
    act(() => toggle.click())
    expect(onToggleArchived).toHaveBeenCalledOnce()

    act(() => root.render(<SessionsSidebar {...props} showArchived totalSessions={1} />))
    expect(container.querySelector('[data-testid="sessions-archive-banner"]')).not.toBeNull()
    expect(container.textContent).toContain('ARCHIVED')
    expect(container.textContent).not.toContain('ACTIVE')

    const row = [...container.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
      button.textContent?.includes('ARCHIVED'),
    )!
    act(() => row.dispatchEvent(new MouseEvent('click', { bubbles: true, ctrlKey: true })))
    const restore = [...container.querySelectorAll<HTMLButtonElement>('button')].find((button) =>
      button.textContent?.includes('Arşivden çıkar'),
    )!
    act(() => restore.click())
    expect(onSetArchived).toHaveBeenCalledWith('ARCHIVED', false)
  })
})

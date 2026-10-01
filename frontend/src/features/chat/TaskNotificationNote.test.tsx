// @vitest-environment jsdom

import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18next } from '@/i18n'
import type { Message } from '@/types'
import { TaskNotificationNote } from './TaskNotificationNote'

const reactTestEnvironment = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT: boolean
}
reactTestEnvironment.IS_REACT_ACT_ENVIRONMENT = true

const roots: Root[] = []

beforeEach(async () => {
  await i18next.changeLanguage('en')
})

function notification(taskId = 'SES9') {
  return `<task-notification>
<task-id>${taskId}</task-id>
<agent-id>AGT9</agent-id>
<agent>Ada</agent>
<status>completed</status>
<summary>İş bitti</summary>
<result>Sonuç gövdesi</result>
<tool_uses>4</tool_uses>
<duration_ms>2500</duration_ms>
</task-notification>`
}

function renderNote(taskId = 'SES9', onSelectSession = vi.fn(), laterStatus?: string) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  const message: Message = {
    id: 'MSG1',
    sessionId: 'SES1',
    role: 'user',
    origin: 'worker-note',
    text: notification(taskId),
    createdAt: 1,
  }
  act(() =>
    root.render(
      <TaskNotificationNote
        message={message}
        onSelectSession={onSelectSession}
        laterStatus={laterStatus}
      />,
    ),
  )
  return { container, onSelectSession }
}

function click(element: Element) {
  act(() => element.dispatchEvent(new MouseEvent('click', { bubbles: true })))
}

afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})

describe('TaskNotificationNote', () => {
  it('shows uncertainty and later failure before expanding an old PASS report', () => {
    const { container } = renderNote('SES51', vi.fn(), 'failed')
    expect(container.querySelector('[data-testid="task-report-evidence"]')?.textContent).toContain(
      'does not establish that tests passed',
    )
    expect(container.querySelector('[data-testid="task-later-report"]')?.textContent).toContain(
      'failed',
    )
    expect(container.textContent).not.toContain('Sonuç gövdesi')
  })
  it('opens the worker session from agent identity without expanding the result', () => {
    const { container, onSelectSession } = renderNote()
    const agentButton = container.querySelector('button[aria-label="Open Ada worker session"]')!

    click(agentButton)

    expect(onSelectSession).toHaveBeenCalledWith('SES9')
    expect(container.textContent).not.toContain('Sonuç gövdesi')
  })

  it('uses the running-worker identity row surface without nesting controls', () => {
    const { container } = renderNote()
    const agentButton = container.querySelector(
      'button[aria-label="Open Ada worker session"]',
    ) as HTMLButtonElement

    expect(agentButton.className).toContain('bg-[var(--color-surface-2)]')
    expect(agentButton.className).toContain('border-[var(--color-border)]')
    expect(agentButton.className).toContain('hover:border-[var(--color-accent)]')
    expect(agentButton.querySelector('button')).toBeNull()
  })

  it('expands the result from the dedicated chevron control', () => {
    const { container } = renderNote()
    const expandButton = container.querySelector('button[aria-label="Expand worker result"]')!

    click(expandButton)

    expect(expandButton.getAttribute('aria-expanded')).toBe('true')
    expect(container.textContent).toContain('Sonuç gövdesi')
  })

  it('places tool count and duration under the right-aligned status', () => {
    const { container } = renderNote()
    const cluster = container.querySelector('[data-testid="task-status-meta"]')!

    expect(cluster.className).toContain('items-end')
    expect(cluster.textContent).toContain('completed')
    expect(cluster.textContent).toContain('4 tools')
    expect(cluster.textContent).toMatch(/2[.,]5 ?s(n)?/)
  })

  it('keeps legacy notifications without task id non-navigable', () => {
    const { container, onSelectSession } = renderNote('')

    expect(container.querySelector('button[aria-label="Open Ada worker session"]')).toBeNull()
    expect(container.textContent).toContain('Ada')
    expect(onSelectSession).not.toHaveBeenCalled()
  })
})

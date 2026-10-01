// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { i18next } from '@/i18n'
import { ExecutionNotice } from './ExecutionNotice'
import { TaskNotificationNote } from './TaskNotificationNote'
import { notificationExecutionSteps } from './executionEvidence'
import { notificationChanges } from './parseTaskNotification'

Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
const roots: Root[] = []
beforeEach(async () => {
  await i18next.changeLanguage('en')
})
afterEach(() => {
  for (const root of roots.splice(0)) act(() => root.unmount())
  document.body.replaceChildren()
})
function render(node: React.ReactNode) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  act(() => root.render(node))
  return container
}

it('keeps interruption visible before details are opened', () => {
  const container = render(
    <ExecutionNotice
      step={{
        kind: 'recovery',
        operation: 'cli_interrupted',
        status: 'timeout',
        durationMs: 300000,
      }}
    />,
  )
  expect(container.querySelector('summary')?.textContent).toContain('Work interrupted — unfinished')
  expect(container.querySelector('details')?.open).toBe(false)
  expect(container.textContent).toContain('Partial work was saved')
})

it('shows delivery evidence while the worker result is collapsed and opens present files only', () => {
  const onOpenFile = vi.fn()
  const container = render(
    <TaskNotificationNote
      onOpenFile={onOpenFile}
      message={{
        id: 'MSG1',
        sessionId: 'SES1',
        role: 'user',
        origin: 'worker-note',
        createdAt: 1,
        text: '<task-notification><task-id>SES2</task-id><status>incomplete</status><result>Still writing files</result></task-notification>',
        steps: JSON.stringify([
          {
            kind: 'text',
            operation: 'delivery_check',
            status: 'missing',
            output: JSON.stringify([
              { path: 'C:/result.md', status: 'present' },
              { path: 'C:/missing.md', status: 'unavailable' },
            ]),
          },
        ]),
      }}
    />,
  )
  expect(container.querySelector('summary')?.textContent).toContain('Delivery incomplete')
  expect(container.textContent).not.toContain('Still writing files')
  const file = [...container.querySelectorAll('button')].find(
    (button) => button.textContent === 'C:/result.md',
  )!
  act(() => file.dispatchEvent(new MouseEvent('click', { bubbles: true })))
  expect(onOpenFile).toHaveBeenCalledWith('C:/result.md')
  expect(
    [...container.querySelectorAll('button')].some(
      (button) => button.textContent === 'C:/missing.md',
    ),
  ).toBe(false)
})

it('explains safe context fallback and preserves recovered connection diagnostics', () => {
  const cold = render(
    <ExecutionNotice
      step={{
        kind: 'text',
        operation: 'cli_resume',
        status: 'disabled',
        reason: 'multiple_responders',
      }}
    />,
  )
  expect(cold.textContent).toContain('responder identity could not be safely reused')
  const recovered = render(
    <ExecutionNotice
      step={{
        kind: 'text',
        operation: 'mcp_startup',
        status: 'recovered',
        target: ['zvec-grep'],
        output: 'handshake failed; token=[redacted]',
      }}
    />,
  )
  expect(recovered.querySelector('summary')?.textContent).toContain('Tool connection recovered')
  expect(recovered.querySelector('pre')?.textContent).toContain('[redacted]')
})

it('handles malformed legacy notification traces without crashing', () => {
  expect(notificationExecutionSteps('{')).toEqual([])
  expect(notificationExecutionSteps('{}')).toEqual([])
  expect(() => notificationChanges('{')).toThrow()
  const container = render(
    <TaskNotificationNote
      message={{
        id: 'legacy',
        sessionId: 'SES1',
        role: 'user',
        createdAt: 1,
        origin: 'worker-note',
        text: 'Legacy worker report',
        steps: '{',
      }}
    />,
  )
  expect(container.textContent).toContain('saved activity details could not be read')
})

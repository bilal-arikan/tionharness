// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { ExecutionNotice } from './ExecutionNotice'

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

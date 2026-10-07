// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { i18next } from '@/i18n'
import { api } from '@/api'
import type { DeciderDebugReport, DeciderView } from '@/types/decider'
import { DeciderDebug } from './DeciderDebug'

vi.mock('@/api', () => ({ api: { getDeciderDebug: vi.fn() } }))
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true
let root: Root
let container: HTMLDivElement
const report: DeciderDebugReport = {
  events: [
    {
      at: 12,
      traceId: 'trace',
      authority: 'stall-judge',
      mode: 'shadow',
      stage: 'outcome',
      role: 'primary',
      threshold: 0.7,
      sessionId: 'SES7',
      outcome: 'ok',
      baseline: 'ok',
    },
    {
      at: 11,
      traceId: 'trace',
      authority: 'stall-judge',
      mode: 'shadow',
      stage: 'completed',
      role: 'primary',
      threshold: 0.7,
      sessionId: 'SES7',
    },
    {
      at: 10,
      traceId: 'trace',
      authority: 'stall-judge',
      mode: 'shadow',
      stage: 'attempt',
      role: 'primary',
      threshold: 0.7,
      sessionId: 'SES7',
      answers: { stalled: { type: 'noul', probability: 0.18 } },
      warnings: ['missing_logprobs'],
    },
  ],
  summary: {
    decisions: 1,
    errors: 0,
    skipped: 0,
    attempts: 1,
    fallbacks: 0,
    challengers: 0,
    retries: 0,
    applied: 0,
    compared: 1,
    agreed: 1,
    challengerCompared: 0,
    challengerAgreed: 0,
    tests: 0,
    testErrors: 0,
    warnings: 1,
    trimmed: 0,
    p50Ms: 10,
    p95Ms: 10,
    costUsd: 0,
  },
  issues: [{ code: 'insufficient_evidence', count: 1 }],
  retainedEvents: 3,
  matchedEvents: 3,
  capacity: 5000,
  truncated: false,
}
const view = { authorities: [], models: [] } as unknown as DeciderView

beforeEach(async () => {
  await i18next.changeLanguage('en')
  vi.mocked(api.getDeciderDebug).mockReset().mockResolvedValue(report)
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  document.body.replaceChildren()
})

it('shows a correlated timeline and distinguishes yes probability from correctness', async () => {
  await act(async () => {
    root.render(<DeciderDebug view={view} />)
  })
  // The evidence caveat sits behind the (ⓘ) on the agreement line.
  expect(container.textContent).not.toContain('Agreement is not accuracy')
  const info = [...container.querySelectorAll<HTMLElement>('[role="button"][aria-expanded]')].find(
    (el) => el.parentElement?.textContent?.includes('agreement'),
  )!
  act(() => info.click())
  expect(document.body.textContent).toContain('Agreement is not accuracy')
  expect(container.textContent).toContain('SES7')
  expect(container.querySelector('summary')?.textContent).toContain('Shadow')
  expect(container.textContent).toContain('P(yes) = 18%')
  expect(container.textContent).toContain('Probabilities unavailable')
  expect(container.querySelectorAll('details')).toHaveLength(1)
  expect(api.getDeciderDebug).toHaveBeenCalledWith({
    days: 7,
    authority: '',
    instance: '',
    ref: '',
    limit: 500,
  })
})

it('surfaces a failed read and retries without changing authority configuration', async () => {
  vi.mocked(api.getDeciderDebug).mockRejectedValueOnce(new Error('offline'))
  await act(async () => {
    root.render(<DeciderDebug view={view} />)
  })
  expect(container.querySelector('[role="alert"]')?.textContent).toContain('offline')
  await act(async () => {
    container
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
  })
  expect(container.querySelector('[role="alert"]')).toBeNull()
  expect(container.textContent).toContain('SES7')
})

it('discloses the bounded history and an empty result', async () => {
  vi.mocked(api.getDeciderDebug).mockResolvedValueOnce({
    ...report,
    events: [],
    matchedEvents: 800,
    truncated: true,
  })
  await act(async () => {
    root.render(<DeciderDebug view={view} />)
  })
  expect(container.textContent).toContain('not lifetime history')
  expect(container.textContent).toContain('Existing ledger records are not reconstructed')
})

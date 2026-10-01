// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { getActiveWorkspace } from '@/api/client'
import { i18next } from '@/i18n'
import type { SessionDebugSummary } from '@/types'
import type { DeciderDebugReport, DeciderView } from '@/types/decider'
import { SessionDebugModal } from './SessionDebugModal'

vi.mock('@/api', () => ({
  api: { getDecider: vi.fn(), getDeciderDebug: vi.fn(), sessionDebugSummary: vi.fn() },
}))
vi.mock('@/api/client', () => ({ getActiveWorkspace: vi.fn() }))
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const view = { authorities: [], models: [] } as unknown as DeciderView
const report: DeciderDebugReport = {
  events: [],
  summary: {
    decisions: 0,
    errors: 0,
    skipped: 0,
    attempts: 0,
    fallbacks: 0,
    challengers: 0,
    retries: 0,
    applied: 0,
    compared: 0,
    agreed: 0,
    challengerCompared: 0,
    challengerAgreed: 0,
    tests: 0,
    testErrors: 0,
    warnings: 0,
    trimmed: 0,
    p50Ms: 0,
    p95Ms: 0,
    costUsd: 0,
  },
  issues: [],
  retainedEvents: 0,
  matchedEvents: 0,
  capacity: 5000,
  truncated: false,
}
let root: Root
let container: HTMLDivElement
const onClose = vi.fn()
async function render(initialTab?: 'runtime' | 'decisions') {
  await act(async () =>
    root.render(
      <SessionDebugModal
        sessionId="SES5"
        title="Example session"
        agentNames={{ AG1: 'Ada' }}
        onClose={onClose}
        initialTab={initialTab}
      />,
    ),
  )
}
const tabs = () => [...container.querySelectorAll<HTMLButtonElement>('[role="tab"]')]

beforeEach(async () => {
  await i18next.changeLanguage('en')
  vi.mocked(api.getDecider).mockReset().mockResolvedValue(view)
  vi.mocked(api.getDeciderDebug).mockReset().mockResolvedValue(report)
  vi.mocked(api.sessionDebugSummary)
    .mockReset()
    .mockResolvedValue({ events: 0 } as SessionDebugSummary)
  vi.mocked(getActiveWorkspace).mockReset().mockReturnValue('WS3')
  onClose.mockReset()
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  document.body.replaceChildren()
})

it('preserves runtime as the default and mounts the real journal for each selected tab', async () => {
  await render()
  expect(api.sessionDebugSummary).toHaveBeenCalledExactlyOnceWith('SES5')
  expect(api.getDecider).not.toHaveBeenCalled()
  expect(tabs()[0].getAttribute('aria-selected')).toBe('true')
  expect(tabs()[1].getAttribute('aria-selected')).toBe('false')
  await act(async () => tabs()[1].click())
  expect(tabs()[1].getAttribute('aria-selected')).toBe('true')
  expect(tabs()[0].getAttribute('aria-selected')).toBe('false')
  expect(container.querySelector('[data-testid="decider-debug"]')).not.toBeNull()
  expect(api.getDeciderDebug).toHaveBeenCalledExactlyOnceWith({
    days: 7,
    authority: '',
    instance: '',
    ref: 'SES5',
    workspaceId: 'WS3',
    limit: 500,
  })
  await act(async () => tabs()[0].click())
  expect(container.querySelector('[data-testid="decider-debug"]')).toBeNull()
  expect(api.sessionDebugSummary).toHaveBeenCalledTimes(2)
  expect(api.getDeciderDebug).toHaveBeenCalledTimes(1)
})

it('opens the decisions tab directly when requested without fetching runtime records', async () => {
  await render('decisions')
  expect(api.sessionDebugSummary).not.toHaveBeenCalled()
  expect(tabs()[1].getAttribute('aria-selected')).toBe('true')
  expect(api.getDeciderDebug).toHaveBeenCalledWith(
    expect.objectContaining({ ref: 'SES5', workspaceId: 'WS3' }),
  )
  expect(container.querySelector('[role="tabpanel"] [data-testid="decider-debug"]')).not.toBeNull()
  expect(container.querySelector('[role="dialog"]')?.textContent).toContain('Example session')
})

it('keeps the close action available from the decisions tab', async () => {
  await render('decisions')
  const closeLabel = i18next.t('actions.close', { ns: 'sessions' })
  const close = [...container.querySelectorAll<HTMLButtonElement>('button')].find(
    (button) => button.getAttribute('aria-label') === closeLabel,
  )!
  act(() => close.click())
  expect(onClose).toHaveBeenCalledOnce()
})

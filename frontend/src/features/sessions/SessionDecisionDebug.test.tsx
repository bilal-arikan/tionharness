// @vitest-environment jsdom
import { act } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { api } from '@/api'
import { getActiveWorkspace } from '@/api/client'
import { i18next } from '@/i18n'
import type { DeciderDebugReport, DeciderView } from '@/types/decider'
import { SessionDecisionDebug } from './SessionDecisionDebug'

vi.mock('@/api', () => ({ api: { getDecider: vi.fn(), getDeciderDebug: vi.fn() } }))
vi.mock('@/api/client', () => ({ getActiveWorkspace: vi.fn() }))
;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const view = { authorities: [], models: [] } as unknown as DeciderView
function report(traceId = ''): DeciderDebugReport {
  return {
    events: traceId
      ? [
          {
            at: 1,
            traceId,
            authority: 'session-setup',
            mode: 'shadow',
            stage: 'completed',
            threshold: 0.8,
          },
        ]
      : [],
    summary: {
      decisions: traceId ? 1 : 0,
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
    retainedEvents: traceId ? 1 : 0,
    matchedEvents: traceId ? 1 : 0,
    capacity: 5000,
    truncated: false,
  }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((yes) => {
    resolve = yes
  })
  return { promise, resolve }
}
let root: Root
let container: HTMLDivElement
async function render(sessionId = 'SES1') {
  await act(async () => root.render(<SessionDecisionDebug sessionId={sessionId} />))
}

beforeEach(async () => {
  await i18next.changeLanguage('en')
  vi.mocked(api.getDecider).mockReset().mockResolvedValue(view)
  vi.mocked(api.getDeciderDebug).mockReset().mockResolvedValue(report())
  vi.mocked(getActiveWorkspace).mockReset().mockReturnValue('WS1')
  container = document.createElement('div')
  document.body.append(container)
  root = createRoot(container)
})
afterEach(() => {
  act(() => root.unmount())
  document.body.replaceChildren()
})

it('loads metadata before requesting the real decision journal scoped to session and workspace', async () => {
  const pending = deferred<DeciderView>()
  vi.mocked(api.getDecider).mockReturnValueOnce(pending.promise)
  await render()
  expect(container.textContent).toContain(i18next.t('decisions.loading', { ns: 'sessions' }))
  expect(api.getDeciderDebug).not.toHaveBeenCalled()
  await act(async () => pending.resolve(view))
  expect(api.getDeciderDebug).toHaveBeenCalledExactlyOnceWith({
    days: 7,
    authority: '',
    instance: '',
    ref: 'SES1',
    workspaceId: 'WS1',
    limit: 500,
  })
  expect(container.querySelector('[data-testid="decider-debug"]')).not.toBeNull()
  const reference = container.querySelector<HTMLInputElement>('input[maxlength="128"]')!
  expect(reference.value).toBe('SES1')
  expect(reference.readOnly).toBe(true)
})

it('shows a metadata failure and recovers through explicit retry', async () => {
  vi.mocked(api.getDecider).mockRejectedValueOnce(new Error('Metadata unavailable'))
  await render()
  expect(container.querySelector('[role="alert"]')?.textContent).toBe('Metadata unavailable')
  expect(api.getDeciderDebug).not.toHaveBeenCalled()
  await act(async () => container.querySelector<HTMLButtonElement>('button')!.click())
  expect(api.getDecider).toHaveBeenCalledTimes(2)
  expect(container.querySelector('[role="alert"]')).toBeNull()
  expect(container.querySelector('[data-testid="decider-debug"]')).not.toBeNull()
  expect(api.getDeciderDebug).toHaveBeenCalledWith(
    expect.objectContaining({ ref: 'SES1', workspaceId: 'WS1' }),
  )
})

it('ignores a pending old-session journal response after switching sessions', async () => {
  const pending = deferred<DeciderDebugReport>()
  vi.mocked(api.getDeciderDebug).mockReturnValueOnce(pending.promise)
  await render()
  vi.mocked(api.getDeciderDebug).mockResolvedValueOnce(report('current-session-trace'))
  await render('SES2')
  expect(api.getDeciderDebug).toHaveBeenLastCalledWith(
    expect.objectContaining({ ref: 'SES2', workspaceId: 'WS1' }),
  )
  await act(async () => pending.resolve(report('old-session-trace')))
  expect(container.textContent).toContain('current-session-trace')
  expect(container.textContent).not.toContain('old-session-trace')
  expect(container.querySelector<HTMLInputElement>('input[maxlength="128"]')!.value).toBe('SES2')
})

it('uses the current session when global metadata resolves after a session change', async () => {
  const pending = deferred<DeciderView>()
  vi.mocked(api.getDecider).mockReturnValueOnce(pending.promise)
  await render()
  await render('SES2')
  expect(api.getDeciderDebug).not.toHaveBeenCalled()
  await act(async () => pending.resolve(view))
  expect(api.getDeciderDebug).toHaveBeenCalledExactlyOnceWith(
    expect.objectContaining({ ref: 'SES2', workspaceId: 'WS1' }),
  )
})

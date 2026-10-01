// @vitest-environment jsdom
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { i18next } from '@/i18n'
import type { SessionDecisions } from '@/api/sessionDecisions'
import { SessionDecisionsCard } from './SessionDecisionsCard'

const api = vi.hoisted(() => ({ read: vi.fn(), pin: vi.fn(), feedback: vi.fn() }))
vi.mock('@/api/sessionDecisions', () => ({ sessionDecisionsApi: api }))

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true
const roots: ReturnType<typeof createRoot>[] = []

function state(sessionId = 'SES1'): SessionDecisions {
  return {
    sessionId,
    entries: [
      {
        id: 'decision-1',
        at: 1760000000000,
        authority: 'startup',
        mode: 'shadow',
        status: 'observed',
        traceId: 'trace-1',
        recommended: 'Recommended skill-a',
        applied: 'Baseline skill-b',
        baseline: 'Existing preparation',
        items: [
          { key: 'tool-a', kind: 'tool', label: 'Tool A', action: 'activate', strength: 0.94 },
        ],
      },
    ],
    memories: [
      {
        key: 'memory-1',
        label: 'Architecture constraint',
        text: 'Retained original context',
        pinned: false,
        mandatory: false,
        addedAtCompact: 1,
        lastReminded: 3,
      },
      {
        key: 'required-1',
        label: 'Required user instruction',
        pinned: true,
        mandatory: true,
        addedAtCompact: 0,
        lastReminded: 0,
      },
    ],
    selectedSkills: ['skill-a'],
    selectedTools: ['tool-a'],
    route: { provider: 'provider-a', model: 'model-a', pinned: false },
    compactCount: 3,
    feedback: [],
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (cause: unknown) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

function buttons(container: HTMLElement) {
  return [...container.querySelectorAll<HTMLButtonElement>('button')]
}

function button(container: HTMLElement, text: string) {
  const found = buttons(container).find((item) => item.textContent?.trim() === text)
  if (!found) throw new Error(`Missing button: ${text}`)
  return found
}

async function mount(sessionId = 'SES1', refreshKey = 0, onError = vi.fn()) {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const root = createRoot(container)
  roots.push(root)
  const render = async (id = sessionId, key = refreshKey) => {
    await act(async () =>
      root.render(<SessionDecisionsCard sessionId={id} refreshKey={key} onError={onError} />),
    )
  }
  await render()
  return { container, render, onError }
}

beforeEach(async () => {
  api.read.mockReset().mockResolvedValue(state())
  api.pin.mockReset()
  api.feedback.mockReset()
  await i18next.changeLanguage('en')
})
afterEach(() => {
  roots.splice(0).forEach((root) => act(() => root.unmount()))
  document.body.replaceChildren()
})

describe('SessionDecisionsCard', () => {
  it('shows actual preparation, context and separate recommendation/application facts', async () => {
    const { container } = await mount()
    expect(container.textContent).toContain('skill-a')
    expect(container.textContent).toContain('tool-a')
    expect(container.textContent).toContain('provider-a')
    expect(container.textContent).toContain('model-a')
    expect(container.textContent).toContain('Retained original context')
    expect(container.textContent).toContain('Recommended skill-a')
    expect(container.textContent).toContain('Baseline skill-b')
    expect(container.textContent).toContain('trace-1')
    expect(container.textContent).toContain('94%')
    const mandatory = container.querySelector<HTMLButtonElement>(
      '[aria-label="Pin context: Required user instruction"]',
    )!
    expect(mandatory.disabled).toBe(true)
    expect(mandatory.getAttribute('aria-pressed')).toBe('true')
  })

  it('writes the model pin once for simultaneous clicks and accepts the returned route', async () => {
    const pending = deferred<SessionDecisions>()
    api.pin.mockReturnValue(pending.promise)
    const { container } = await mount()
    const pin = button(container, 'Pin')
    act(() => {
      pin.click()
      pin.click()
    })
    expect(api.pin).toHaveBeenCalledExactlyOnceWith('SES1', 'model', true)
    expect(pin.disabled).toBe(true)
    const updated = state()
    updated.route!.pinned = true
    api.read.mockResolvedValue(updated)
    await act(async () => pending.resolve(updated))
    expect(button(container, 'Unpin').getAttribute('aria-pressed')).toBe('true')
  })

  it('records feedback with the decision id and shows the server-confirmed rating', async () => {
    const updated = state()
    updated.feedback = [{ decisionId: 'decision-1', rating: 'helpful', at: 1760000000001 }]
    api.feedback.mockResolvedValue(updated)
    const { container } = await mount()
    api.read.mockResolvedValue(updated)
    await act(async () => button(container, 'Helpful').click())
    expect(api.feedback).toHaveBeenCalledExactlyOnceWith('SES1', 'decision-1', 'helpful')
    expect(button(container, 'Helpful').getAttribute('aria-pressed')).toBe('true')
    expect(button(container, 'Needs correction').getAttribute('aria-pressed')).toBe('false')
  })

  it('aborts and ignores a stale read after switching sessions', async () => {
    const pending = deferred<SessionDecisions>()
    api.read.mockReturnValueOnce(pending.promise)
    const { container, render } = await mount()
    const signal = api.read.mock.calls[0][1] as AbortSignal
    const next = state('SES2')
    next.selectedSkills = ['session-two-only']
    next.entries = []
    api.read.mockResolvedValue(next)
    await render('SES2')
    expect(signal.aborted).toBe(true)
    await act(async () => pending.resolve(state()))
    expect(container.textContent).toContain('session-two-only')
    expect(container.textContent).not.toContain('skill-a')
  })

  it('ignores a completed old-session mutation and leaves the new session actionable', async () => {
    const pending = deferred<SessionDecisions>()
    api.pin.mockReturnValue(pending.promise)
    const { container, render } = await mount()
    act(() => button(container, 'Pin').click())
    const next = state('SES2')
    next.route = { provider: 'provider-b', model: 'model-b', pinned: false }
    api.read.mockResolvedValue(next)
    await render('SES2')
    const old = state()
    old.route!.pinned = true
    await act(async () => pending.resolve(old))
    expect(container.textContent).toContain('model-b')
    expect(container.textContent).not.toContain('model-a')
    expect(button(container, 'Pin').disabled).toBe(false)
    expect(api.pin).toHaveBeenCalledExactlyOnceWith('SES1', 'model', true)
  })

  it('disables writes during a refresh and restores them only after its response', async () => {
    const { container, render } = await mount()
    const pending = deferred<SessionDecisions>()
    api.read.mockReturnValueOnce(pending.promise)
    await render('SES1', 1)
    expect(button(container, 'Pin').disabled).toBe(true)
    expect(button(container, 'Helpful').disabled).toBe(true)
    const updated = state()
    updated.selectedTools = ['fresh-tool']
    await act(async () => pending.resolve(updated))
    expect(container.textContent).toContain('fresh-tool')
    expect(button(container, 'Pin').disabled).toBe(false)
  })

  it('displays a read error and recovers through explicit retry without fabricated state', async () => {
    api.read.mockRejectedValueOnce(new Error('Decision service unavailable'))
    const { container, onError } = await mount()
    expect(container.querySelector('[role="alert"]')?.textContent).toBe(
      'Decision service unavailable',
    )
    expect(onError).toHaveBeenCalledWith('Decision service unavailable')
    expect(container.textContent).not.toContain('model-a')
    await act(async () => button(container, 'Refresh').click())
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(container.textContent).toContain('model-a')
  })

  it('keeps a failed mutation visible even when the following background read succeeds', async () => {
    api.pin.mockRejectedValue(new Error('Pin could not be saved'))
    const { container } = await mount()
    await act(async () => button(container, 'Pin').click())
    expect(container.querySelector('[role="alert"]')?.textContent).toBe('Pin could not be saved')
    expect(button(container, 'Pin').disabled).toBe(true)
    await act(async () => button(container, 'Refresh').click())
    expect(container.querySelector('[role="alert"]')).toBeNull()
    expect(button(container, 'Pin').disabled).toBe(false)
  })
})

// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearActiveWorkspace, setActiveWorkspace } from './client'
import { sessionDecisionsApi, type SessionDecisions } from './sessionDecisions'

const snapshot: SessionDecisions = {
  sessionId: 'SES /?1',
  entries: [],
  memories: [],
  selectedSkills: [],
  selectedTools: [],
  compactCount: 0,
  feedback: [],
}

beforeEach(() => setActiveWorkspace('WS-test'))
afterEach(() => {
  clearActiveWorkspace()
  vi.restoreAllMocks()
})

describe('session decision HTTP contract', () => {
  it('encodes the session id and scopes reads to the active workspace', async () => {
    const fetch = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify(snapshot)))
    await expect(sessionDecisionsApi.read(snapshot.sessionId)).resolves.toEqual(snapshot)
    expect(fetch.mock.calls[0][0]).toBe('/api/sessions/SES%20%2F%3F1/decisions')
    expect(fetch.mock.calls[0][1]).toMatchObject({
      cache: 'no-store',
      headers: { 'Content-Type': 'application/json', 'X-Workspace-Id': 'WS-test' },
    })
  })

  it('sends the model pin and feedback payloads and uses the returned server state', async () => {
    const updated = {
      ...snapshot,
      route: { provider: 'provider-a', model: 'model-a', pinned: true },
    }
    const fetch = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(async () => new Response(JSON.stringify(updated)))
    await expect(sessionDecisionsApi.pin(snapshot.sessionId, 'model', true)).resolves.toEqual(
      updated,
    )
    expect(fetch.mock.calls[0][0]).toBe('/api/sessions/SES%20%2F%3F1/decisions/pin')
    expect(fetch.mock.calls[0][1]?.method).toBe('PUT')
    expect(JSON.parse(String(fetch.mock.calls[0][1]?.body))).toEqual({ key: 'model', pinned: true })
    await expect(
      sessionDecisionsApi.feedback(snapshot.sessionId, 'decision-1', 'correction', 'Wrong route'),
    ).resolves.toEqual(updated)
    expect(fetch.mock.calls[1][0]).toBe('/api/sessions/SES%20%2F%3F1/decisions/feedback')
    expect(fetch.mock.calls[1][1]?.method).toBe('POST')
    expect(JSON.parse(String(fetch.mock.calls[1][1]?.body))).toEqual({
      decisionId: 'decision-1',
      rating: 'correction',
      note: 'Wrong route',
    })
    expect(fetch.mock.calls[1][1]?.headers).toMatchObject({ 'X-Workspace-Id': 'WS-test' })
  })

  it('preserves server validation errors and forwards read cancellation', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: 'Unknown decision' }), { status: 400 }),
    )
    await expect(sessionDecisionsApi.feedback('SES1', 'missing', 'helpful')).rejects.toThrow(
      'Unknown decision',
    )
    const controller = new AbortController()
    const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(init.signal?.reason))
        }),
    )
    fetch.mockClear()
    const pending = sessionDecisionsApi.read('SES1', controller.signal)
    const cancelled = expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    controller.abort()
    await cancelled
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true)
  })
})

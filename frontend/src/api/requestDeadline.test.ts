// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { req } from './client'
import { API_REQUEST_TIMEOUT_MS } from './requestDeadline'
import { i18next } from '@/i18n'

beforeEach(async () => {
  vi.useFakeTimers()
  await i18next.changeLanguage('en')
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

it('releases a stalled fetch with an actionable timeout and aborts the request', async () => {
  let signal: AbortSignal | null | undefined
  vi.spyOn(globalThis, 'fetch').mockImplementation((_url, init) => {
    signal = init?.signal
    return new Promise((_resolve, reject) =>
      signal?.addEventListener('abort', () => reject(signal?.reason)),
    )
  })
  const pending = req('/api/workspaces')
  const result = expect(pending).rejects.toThrow('too long')
  await vi.advanceTimersByTimeAsync(API_REQUEST_TIMEOUT_MS)
  await result
  expect(signal?.aborted).toBe(true)
  expect(vi.getTimerCount()).toBe(0)
})

it('bounds a stalled response body, not only the response headers', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(
    async (_url, init) =>
      new Response(
        new ReadableStream({
          start(controller) {
            init?.signal?.addEventListener('abort', () => controller.error(init.signal?.reason))
          },
        }),
      ),
  )
  const result = expect(req('/api/sessions')).rejects.toThrow('too long')
  await vi.advanceTimersByTimeAsync(API_REQUEST_TIMEOUT_MS)
  await result
  expect(vi.getTimerCount()).toBe(0)
})

it('preserves caller cancellation and cleans up the deadline after success', async () => {
  const controller = new AbortController()
  vi.spyOn(globalThis, 'fetch').mockImplementation(
    (_url, init) =>
      new Promise((_resolve, reject) => {
        init?.signal?.addEventListener('abort', () => reject(init.signal?.reason))
      }),
  )
  const reason = new DOMException('Session changed', 'AbortError')
  const result = expect(req('/api/sessions', { signal: controller.signal })).rejects.toBe(reason)
  controller.abort(reason)
  await result
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response('{}'))
  await expect(req('/api/workspaces')).resolves.toEqual({})
  expect(vi.getTimerCount()).toBe(0)
})

it('keeps blocking runtime endpoints outside the ordinary request deadline', async () => {
  let finish: (response: Response) => void = () => {}
  const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve
      }),
  )
  const pending = req('/api/flows/FLW1/run', { method: 'POST', timeoutMs: 0 })
  await vi.advanceTimersByTimeAsync(API_REQUEST_TIMEOUT_MS * 2)
  expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(false)
  finish(new Response('{}'))
  await expect(pending).resolves.toEqual({})
})

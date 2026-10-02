// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LiveChannelSpec } from './liveConnection'

let live: typeof import('./liveConnection')
const releases: (() => void)[] = []
const streams: ReadableStreamDefaultController<Uint8Array>[] = []
const encoder = new TextEncoder()

beforeEach(async () => {
  vi.resetModules()
  vi.useFakeTimers()
  live = await import('./liveConnection')
})

afterEach(async () => {
  releases.splice(0).forEach((release) => release())
  streams.length = 0
  await vi.advanceTimersByTimeAsync(0)
  vi.useRealTimers()
  vi.restoreAllMocks()
})

function mockStreams() {
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (_url, init) => {
    const body = new ReadableStream<Uint8Array>({
      start(controller) {
        streams.push(controller)
        init?.signal?.addEventListener(
          'abort',
          () => {
            try {
              controller.error(new DOMException('Disconnected', 'AbortError'))
            } catch {
              /* closed */
            }
          },
          { once: true },
        )
      },
    })
    return new Response(body)
  })
}

function add(spec: LiveChannelSpec, onFrame = vi.fn()) {
  releases.push(live.subscribeLiveChannel(() => spec, { onFrame }))
  return onFrame
}

function frame(channel: string, raw: string) {
  return `event: live\ndata: ${JSON.stringify({ channel, frame: raw })}\n\n`
}

describe('one live transport per tab', () => {
  it('batches global, workspace and session feeds into one connection and routes fragmented frames', async () => {
    const fetch = mockStreams()
    const global = add({ key: 'global', scope: 'global' })
    const workspace = add({ key: 'WS1', scope: 'workspace', workspaceId: 'WS1' })
    const session = add({
      key: 'WS1:SES1',
      scope: 'session',
      workspaceId: 'WS1',
      sessionId: 'SES1',
    })
    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledTimes(1)
    const query = new URL(String(fetch.mock.calls[0][0]), 'http://local').searchParams
    expect(JSON.parse(query.get('channels')!)).toHaveLength(3)
    expect(live.getLiveConnectionStatus()).toBe('connected')
    const raw = 'event: hub\ndata: {"seq":1}\n\n'
    const data = ': connected\n\n' + frame('WS1:SES1', raw)
    streams[0].enqueue(encoder.encode(data.slice(0, 25)))
    streams[0].enqueue(encoder.encode(data.slice(25)))
    await vi.advanceTimersByTimeAsync(0)
    expect(session).toHaveBeenCalledExactlyOnceWith(raw)
    expect(global).not.toHaveBeenCalled()
    expect(workspace).not.toHaveBeenCalled()
  })

  it('aborts the old connection before replacing a session and keeps notifications subscribed', async () => {
    const fetch = mockStreams()
    const global = add({ key: 'global', scope: 'global' })
    add({ key: 'SES1', scope: 'session', workspaceId: 'WS1', sessionId: 'SES1' })
    await vi.advanceTimersByTimeAsync(0)
    releases.pop()!()
    add({ key: 'SES2', scope: 'session', workspaceId: 'WS1', sessionId: 'SES2' })
    expect(fetch.mock.calls[0][1]?.signal?.aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledTimes(2)
    expect(String(fetch.mock.calls[1][0])).not.toContain('SES1')
    streams[1].enqueue(encoder.encode(frame('global', 'event: notify\ndata: {"type":"chat"}\n\n')))
    await vi.advanceTimersByTimeAsync(0)
    expect(global).toHaveBeenCalledTimes(1)
  })

  it('retries a rejected connection, carries fresh cursors, and cancels idle retries', async () => {
    const fetch = mockStreams()
    fetch.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    const spec: LiveChannelSpec = {
      key: 'SES1',
      scope: 'session',
      workspaceId: 'WS1',
      sessionId: 'SES1',
      since: 7,
      epoch: 'boot1',
    }
    add(spec)
    await vi.advanceTimersByTimeAsync(0)
    expect(fetch).toHaveBeenCalledTimes(1)
    spec.since = 9
    await vi.advanceTimersByTimeAsync(500)
    expect(fetch).toHaveBeenCalledTimes(2)
    const query = new URL(String(fetch.mock.calls[1][0]), 'http://local').searchParams
    expect(JSON.parse(query.get('channels')!)[0]).toMatchObject({ since: 9, epoch: 'boot1' })
    streams[0].error(new TypeError('Connection lost'))
    await vi.advanceTimersByTimeAsync(0)
    expect(live.getLiveConnectionStatus()).toBe('reconnecting')
    releases.pop()!()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(fetch).toHaveBeenCalledTimes(2)
  })
})
